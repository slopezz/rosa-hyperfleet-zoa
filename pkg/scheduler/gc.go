package scheduler

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/labels"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
)

// RunGC performs only the garbage collection phases. Called by the dedicated
// "gc" EventBridge schedule (every 5m) as a catch-up for resources the
// reconciler's 1-minute tick didn't fully process due to batch limits.
func (r *Reconciler) RunGC(ctx context.Context) error {
	start := time.Now()
	r.logger.Info("garbage collector starting (dedicated GC tick)")

	var phaseErrors int
	if err := r.garbageCollect(ctx); err != nil {
		r.logger.Error("garbage collection failed", "error", err)
		phaseErrors++
	}
	if err := r.orphanGC(ctx); err != nil {
		r.logger.Error("orphan garbage collection failed", "error", err)
		phaseErrors++
	}
	if err := r.cleanupStaleMustGatherPods(ctx); err != nil {
		r.logger.Error("stale must-gather pod cleanup failed", "error", err)
		phaseErrors++
	}

	elapsed := time.Since(start)
	r.logger.Info("garbage collector completed", "duration_ms", elapsed.Milliseconds(), "phase_errors", phaseErrors)

	metrics.EmitGC(r.cfg.TargetCluster, elapsed.Milliseconds(), phaseErrors)

	// Returning an error causes the Lambda invocation to report failure, which
	// surfaces as AWS CloudWatch Lambda Errors metric. This enables native AWS
	// monitoring and alarms without requiring custom metric queries.
	if phaseErrors > 0 {
		return fmt.Errorf("GC completed with %d phase errors", phaseErrors)
	}
	return nil
}

// garbageCollect cleans up K8s resources (Jobs, ServiceAccounts, Roles,
// RoleBindings) for executions in terminal state (succeeded/failed/timed_out).
// Waits gcDelay after terminal transition to allow artifact retrieval.
func (r *Reconciler) garbageCollect(ctx context.Context) error {
	const gcDelay = 5 * time.Minute

	terminal, err := r.executionStore.QueryTerminalByTarget(ctx, r.cfg.TargetCluster, gcDelay)
	if err != nil {
		return err
	}

	var cleaned int
	for i, exec := range terminal {
		if i >= r.cfg.MaxBatchPerTick || ctx.Err() != nil {
			break
		}
		action, ok := actions.Get(exec.Action)
		var rbac *actions.RBACConfig
		if ok {
			rbac = action.Metadata().RBAC
		}

		r.executor.CleanupExecution(ctx, exec.ID, rbac, exec.Params)

		if err := r.executionStore.MarkCleaned(ctx, exec.ID); err != nil {
			r.logger.Warn("failed to mark as cleaned (may already be cleaned)", "execution_id", exec.ID, "error", err)
			continue
		}
		metrics.EmitGCCleaned(r.cfg.TargetCluster, "execution")
		cleaned++
	}

	if cleaned > 0 {
		r.logger.Info("garbage collected executions", "count", cleaned)
	}
	return nil
}

// orphanGC deletes K8s resources (Jobs, ServiceAccounts, Roles, RoleBindings)
// in the zoa-jobs namespace that have no matching DynamoDB execution record.
// This handles crash scenarios where DynamoDB writes failed but K8s resources
// were already created, or where records expired via TTL.
func (r *Reconciler) orphanGC(ctx context.Context) error {
	const orphanAge = 30 * time.Minute

	jobs, err := r.kubeClient.BatchV1().Jobs(r.cfg.JobsNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.ZOAManagedSelector(),
	})
	if err != nil {
		return fmt.Errorf("listing zoa jobs: %w", err)
	}

	var cleaned int
	for i, job := range jobs.Items {
		if i >= r.cfg.MaxBatchPerTick || ctx.Err() != nil {
			break
		}
		if job.CreationTimestamp.After(time.Now().Add(-orphanAge)) {
			continue
		}

		execID := job.Labels[labels.KeyExecutionID]
		if execID == "" {
			continue
		}

		exec, err := r.executionStore.Get(ctx, execID)
		if err != nil {
			r.logger.Warn("failed to check execution for orphan job", "execution_id", execID, "error", err)
			continue
		}

		if exec != nil {
			continue
		}

		r.logger.Warn("deleting orphan K8s resources (no DynamoDB record)", "execution_id", execID, "job", job.Name)
		r.executor.CleanupExecution(ctx, execID, nil, nil)
		metrics.EmitGCCleaned(r.cfg.TargetCluster, "job")
		cleaned++
	}

	if cleaned > 0 {
		r.logger.Info("orphan garbage collection completed", "cleaned", cleaned)
	}
	return nil
}

// cleanupStaleMustGatherPods deletes terminal must-gather child pods left behind when
// the runner Job was killed before its defer cleanup ran. Only Succeeded/Failed pods
// older than staleMustGatherPodAge are removed — Running pods are never touched.
func (r *Reconciler) cleanupStaleMustGatherPods(ctx context.Context) error {
	const staleMustGatherPodAge = 30 * time.Minute

	pods, err := r.kubeClient.CoreV1().Pods(r.cfg.JobsNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.MustGatherPodSelector(),
	})
	if err != nil {
		return fmt.Errorf("listing must-gather pods: %w", err)
	}

	var cleaned int
	for i, pod := range pods.Items {
		if i >= r.cfg.MaxBatchPerTick || ctx.Err() != nil {
			break
		}
		switch pod.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
		default:
			continue
		}
		if pod.CreationTimestamp.After(time.Now().Add(-staleMustGatherPodAge)) {
			continue
		}

		execID := pod.Labels[labels.KeyExecutionID]
		r.logger.Warn("deleting stale must-gather pod",
			"pod", pod.Name,
			"execution_id", execID,
			"phase", pod.Status.Phase,
		)
		if err := r.kubeClient.CoreV1().Pods(r.cfg.JobsNamespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			r.logger.Warn("failed to delete stale must-gather pod", "pod", pod.Name, "error", err)
			continue
		}
		metrics.EmitGCCleaned(r.cfg.TargetCluster, "pod")
		cleaned++
	}

	if cleaned > 0 {
		r.logger.Info("stale must-gather pod cleanup completed", "cleaned", cleaned)
	}
	return nil
}
