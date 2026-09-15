package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

// LambdaInvoker abstracts Lambda invocation for testability.
type LambdaInvoker interface {
	Invoke(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
}

// WorkerExecutionEvent is the payload for self-invocation: reconciler invokes
// the same Worker Lambda to execute a single approved TA.
type WorkerExecutionEvent struct {
	Route       string `json:"route"`
	ExecutionID string `json:"execution_id"`
	Action      string `json:"action"`
}

type Reconciler struct {
	executionStore store.ExecutionStore
	kubeClient     kubernetes.Interface
	lambdaClient   LambdaInvoker
	executor       *executor.Executor
	cfg            *config.Config
	logger         *slog.Logger
}

func NewReconciler(executionStore store.ExecutionStore, kubeClient kubernetes.Interface, lambdaClient LambdaInvoker, exec *executor.Executor, cfg *config.Config, logger *slog.Logger) *Reconciler {
	return &Reconciler{
		executionStore: executionStore,
		kubeClient:     kubeClient,
		lambdaClient:   lambdaClient,
		executor:       exec,
		cfg:            cfg,
		logger:         logger,
	}
}

// Run performs the reconciliation loop:
// 1. Fan-out: dispatch approved TAs by async-invoking the Execution Lambda
// 2. Poll async Jobs: check K8s Job status, transition dispatched→succeeded/failed
// 3. Timeout: mark dispatched executions that exceeded their timeout
// 4. GC: clean up K8s resources for terminal executions, mark cleaned
// 5. Orphan GC: delete K8s resources that have no matching DynamoDB record
// 6. Stale must-gather pods: delete terminal child pods missed by runner defer cleanup
func (r *Reconciler) Run(ctx context.Context) error {
	start := time.Now()
	r.logger.Info("reconciler starting")

	var phaseErrors int

	if err := r.dispatchApproved(ctx); err != nil {
		r.logger.Error("approved dispatch failed", "error", err)
		phaseErrors++
	}

	if err := r.pollAsyncJobs(ctx); err != nil {
		r.logger.Error("async job polling failed", "error", err)
		phaseErrors++
	}

	if err := r.timeoutDispatched(ctx); err != nil {
		r.logger.Error("timeout check failed", "error", err)
		phaseErrors++
	}

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
	r.logger.Info("reconciler completed",
		"duration_ms", elapsed.Milliseconds(),
		"phase_errors", phaseErrors,
	)

	metrics.EmitReconciler(r.cfg.TargetCluster, elapsed.Milliseconds(), phaseErrors)

	// Returning an error causes the Lambda invocation to report failure, which
	// surfaces as AWS CloudWatch Lambda Errors metric. This enables native AWS
	// monitoring and alarms without requiring custom metric queries.
	if phaseErrors > 0 {
		return fmt.Errorf("reconciler completed with %d phase errors", phaseErrors)
	}
	return nil
}

// dispatchApproved scans for TAs in `approved` status and self-invokes the
// Worker Lambda asynchronously for each (fan-out). The conditional write ensures
// only one reconciler tick can claim each execution. A dispatch failure budget
// prevents a broken Lambda from consuming all approved TAs with no actual execution.
// Respects context deadline and MaxBatchPerTick to prevent timeout under load.
func (r *Reconciler) dispatchApproved(ctx context.Context) error {
	const maxDispatchFailures = 3

	approved, err := r.executionStore.QueryByTargetAndStatus(ctx, r.cfg.TargetCluster, store.StatusApproved)
	if err != nil {
		return err
	}
	if len(approved) == 0 {
		return nil
	}

	var dispatched, failures int
	for i, exec := range approved {
		if i >= r.cfg.MaxBatchPerTick {
			r.logger.Info("batch limit reached, rest deferred to next tick", "remaining", len(approved)-i)
			break
		}
		if ctx.Err() != nil {
			r.logger.Warn("deadline approaching, deferring remaining approved TAs", "processed", i, "remaining", len(approved)-i)
			break
		}
		if failures >= maxDispatchFailures {
			r.logger.Warn("dispatch failure budget exhausted, deferring remaining approved TAs",
				"remaining", len(approved)-dispatched-failures)
			break
		}

		if err := r.executionStore.TransitionStatus(ctx, exec.ID, store.StatusApproved, store.StatusDispatched); err != nil {
			r.logger.Debug("failed to claim approved execution (likely another tick claimed it)",
				"execution_id", exec.ID, "error", err)
			continue
		}

		payload, err := json.Marshal(WorkerExecutionEvent{
			Route:       "execute",
			ExecutionID: exec.ID,
			Action:      exec.Action,
		})
		if err != nil {
			r.logger.Error("failed to marshal execution event", "execution_id", exec.ID, "error", err)
			if rbErr := r.executionStore.TransitionStatus(ctx, exec.ID, store.StatusDispatched, store.StatusApproved); rbErr != nil {
				r.logger.Error("failed to rollback after marshal failure", "execution_id", exec.ID, "error", rbErr)
			}
			failures++
			continue
		}

		// Self-invoke: worker Lambda invokes itself with InvocationType=Event (async).
		// AWS_LAMBDA_FUNCTION_NAME is automatically set by the Lambda runtime.
		_, err = r.lambdaClient.Invoke(ctx, &lambda.InvokeInput{
			FunctionName:   aws.String(r.cfg.WorkerFunctionName),
			InvocationType: lambdatypes.InvocationTypeEvent,
			Payload:        payload,
		})
		if err != nil {
			r.logger.Error("failed to self-invoke for TA execution",
				"execution_id", exec.ID, "error", err)
			if rbErr := r.executionStore.TransitionStatus(ctx, exec.ID, store.StatusDispatched, store.StatusApproved); rbErr != nil {
				r.logger.Error("failed to rollback dispatch status after invoke failure",
					"execution_id", exec.ID, "error", rbErr)
			} else {
				r.logger.Info("rolled back dispatch status after invoke failure",
					"execution_id", exec.ID)
			}
			failures++
			continue
		}

		r.logger.Info("dispatched approved TA via self-invoke",
			"execution_id", exec.ID, "action", exec.Action)
		dispatched++
	}

	if dispatched > 0 || failures > 0 {
		r.logger.Info("fan-out dispatch completed", "dispatched", dispatched, "failures", failures, "total_approved", len(approved))
	}
	return nil
}

func (r *Reconciler) timeoutDispatched(ctx context.Context) error {
	dispatched, err := r.executionStore.QueryByTargetAndStatus(ctx, r.cfg.TargetCluster, store.StatusDispatched)
	if err != nil {
		return err
	}

	var timedOut int
	for i, exec := range dispatched {
		if i >= r.cfg.MaxBatchPerTick || ctx.Err() != nil {
			break
		}

		timeout := time.Duration(exec.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = time.Duration(r.cfg.ExecutionDeadlineSeconds) * time.Second
		}

		// Async executions get extra scheduling overhead added to their timeout.
		// The TA's TimeoutSeconds is the actual execution budget. The overhead
		// accounts for GSI propagation + reconciler cadence + Job scheduling —
		// platform concerns invisible to TA authors.
		if exec.ExecutionMode == "async" {
			timeout += time.Duration(r.cfg.AsyncSchedulingOverheadSeconds) * time.Second
		}

		dispatchedAt := exec.DispatchedAt
		if dispatchedAt == "" {
			dispatchedAt = exec.CreatedAt
		}

		t, err := time.Parse(time.RFC3339Nano, dispatchedAt)
		if err != nil {
			r.logger.Warn("failed to parse dispatchedAt", "execution_id", exec.ID, "error", err)
			continue
		}

		if time.Since(t) > timeout {
			r.logger.Warn("execution exceeded timeout, marking as timed_out",
				"execution_id", exec.ID,
				"age", time.Since(t).String(),
				"timeout", timeout.String())

			if err := r.executionStore.TransitionStatus(ctx, exec.ID, store.StatusDispatched, store.StatusTimedOut); err != nil {
				r.logger.Error("failed to transition to timed_out", "execution_id", exec.ID, "error", err)
				continue
			}
			metrics.EmitExecution(r.cfg.TargetCluster, exec.Action, string(store.StatusTimedOut), exec.ExecutionMode, exec.Scope, exec.Type, time.Since(t).Milliseconds())

			if exec.ExecutionMode == "async" {
				jobName := "zoa-" + exec.ID
				background := metav1.DeletePropagationBackground
				if err := r.kubeClient.BatchV1().Jobs(r.cfg.JobsNamespace).Delete(ctx, jobName, metav1.DeleteOptions{
					PropagationPolicy: &background,
				}); err != nil {
					r.logger.Warn("failed to delete timed-out job (may not exist)", "execution_id", exec.ID, "error", err)
				}
			}

			timedOut++
		}
	}

	if timedOut > 0 {
		r.logger.Info("timed out executions", "count", timedOut)
	}
	return nil
}

func (r *Reconciler) pollAsyncJobs(ctx context.Context) error {
	allDispatched, err := r.executionStore.QueryByTargetAndStatus(ctx, r.cfg.TargetCluster, store.StatusDispatched)
	if err != nil {
		return err
	}

	asyncDispatched := make([]*store.Execution, 0, len(allDispatched))
	for _, exec := range allDispatched {
		if exec.ExecutionMode == "async" {
			asyncDispatched = append(asyncDispatched, exec)
		}
	}

	var completed int
	for i, exec := range asyncDispatched {
		if i >= r.cfg.MaxBatchPerTick || ctx.Err() != nil {
			break
		}
		jobName := "zoa-" + exec.ID
		job, err := r.kubeClient.BatchV1().Jobs(r.cfg.JobsNamespace).Get(ctx, jobName, metav1.GetOptions{})
		if err != nil {
			r.logger.Debug("job not found (may not be created yet)", "execution_id", exec.ID, "error", err)
			continue
		}

		status := jobStatus(job)
		if status == "" {
			continue
		}

		var durationMs int64
		if exec.DispatchedAt != "" {
			if t, err := exec.DispatchedAtTime(); err == nil {
				durationMs = time.Since(t).Milliseconds()
			}
		}

		updates := map[string]interface{}{
			"durationMs": durationMs,
		}

		if r.executor != nil {
			outBytes, lgBytes, outFormat := r.executor.ArtifactSizes(ctx, exec.ID)
			if outBytes > 0 {
				updates["outputBytes"] = outBytes
				updates["outputFormat"] = outFormat
			}
			if lgBytes > 0 {
				updates["logBytes"] = lgBytes
			}
		}

		if err := r.executionStore.TransitionWithMetadata(ctx, exec.ID, store.StatusDispatched, status, updates); err != nil {
			r.logger.Warn("failed to transition async execution", "execution_id", exec.ID, "target_status", status, "error", err)
			continue
		}
		metrics.EmitExecution(r.cfg.TargetCluster, exec.Action, string(status), exec.ExecutionMode, exec.Scope, exec.Type, durationMs)

		r.logger.Info("async execution completed", "execution_id", exec.ID, "status", status, "duration_ms", durationMs)
		completed++
	}

	if completed > 0 {
		r.logger.Info("polled async jobs", "completed", completed, "checked", len(asyncDispatched))
	}
	return nil
}

func jobStatus(job *batchv1.Job) store.Status {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == "True" {
			return store.StatusSucceeded
		}
		if c.Type == batchv1.JobFailed && c.Status == "True" {
			return store.StatusFailed
		}
	}
	return ""
}
