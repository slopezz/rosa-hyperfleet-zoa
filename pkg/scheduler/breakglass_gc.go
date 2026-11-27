package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/labels"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/metrics"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// breakglassExpireGrants transitions pending/active requests past deadline to status=expired.
func (r *Reconciler) breakglassExpireGrants(ctx context.Context) error {
	now := time.Now()
	for _, status := range []store.Status{store.StatusPending, store.StatusActive} {
		items, err := r.breakglassStore.QueryByTargetAndStatus(ctx, r.cfg.TargetCluster, status)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := r.breakglassExpireIfNeeded(ctx, item, now); err != nil {
				r.logger.Error("breakglass expire", "pillar", PillarBreakglass, "phase", "expire_grants", "requestId", item.RequestID, "error", err)
			}
		}
	}
	return nil
}

// breakglassGarbageCollect removes kube RBAC for expired grants and marks rows cleaned.
func (r *Reconciler) breakglassGarbageCollect(ctx context.Context) error {
	if r.executor == nil {
		return nil
	}
	items, err := r.breakglassStore.QueryByTargetAndStatus(ctx, r.cfg.TargetCluster, store.StatusExpired)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Cleaned {
			continue
		}
		if def, ok := breakglass.Get(item.Profile); ok && def.Metadata.Scope == breakglass.ScopeKube {
			r.executor.CleanupBreakglass(ctx, item.RequestID)
		}
		if err := r.breakglassStore.MarkCleaned(ctx, item.RequestID); err != nil {
			r.logger.Error("breakglass mark cleaned", "pillar", PillarBreakglass, "phase", "garbage_collect", "requestId", item.RequestID, "error", err)
		}
	}
	return nil
}

const breakglassOrphanAge = 30 * time.Minute

// breakglassOrphanGC deletes zoa-bg kube objects when Dynamo has no row or the grant is no longer pending/active.
func (r *Reconciler) breakglassOrphanGC(ctx context.Context) error {
	if r.executor == nil {
		return nil
	}
	kube := r.executor.Kubernetes()
	if kube == nil {
		return nil
	}
	ns := labels.JobsNamespace()
	list, err := kube.CoreV1().ServiceAccounts(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.BreakglassResourceSelector(),
	})
	if err != nil {
		return fmt.Errorf("listing break-glass service accounts: %w", err)
	}
	cutoff := time.Now().Add(-breakglassOrphanAge)
	var cleaned int
	for _, sa := range list.Items {
		if sa.CreationTimestamp.After(cutoff) {
			continue
		}
		requestID := strings.TrimSpace(sa.Labels[labels.KeyRequestID])
		if requestID == "" {
			requestID = breakglassRequestIDFromSAName(sa.Name)
		}
		if requestID == "" {
			continue
		}
		req, err := r.breakglassStore.Get(ctx, requestID)
		if err != nil {
			r.logger.Warn("breakglass orphan check: dynamodb get failed", "requestId", requestID, "error", err)
			continue
		}
		if !shouldOrphanBreakglassKube(req, r.cfg.TargetCluster) {
			continue
		}
		r.logger.Warn("deleting orphan break-glass kube resources",
			"requestId", requestID,
			"serviceAccount", sa.Name,
			"dynamo_missing", req == nil,
		)
		r.executor.CleanupBreakglass(ctx, requestID)
		if req != nil && req.Status == store.StatusExpired && !req.Cleaned {
			if err := r.breakglassStore.MarkCleaned(ctx, requestID); err != nil {
				r.logger.Error("breakglass mark cleaned after orphan gc", "pillar", PillarBreakglass, "phase", "orphan_gc", "requestId", requestID, "error", err)
			}
		}
		metrics.EmitGCCleaned(r.cfg.TargetCluster, "breakglass")
		cleaned++
	}
	if cleaned > 0 {
		r.logger.Info("break-glass orphan garbage collection completed", "cleaned", cleaned)
	}
	return nil
}

func (r *Reconciler) breakglassExpireIfNeeded(ctx context.Context, item *store.BreakglassRequest, now time.Time) error {
	var cutoff string
	switch item.Status {
	case store.StatusPending:
		cutoff = item.RequestExpiresAt
	case store.StatusActive:
		cutoff = item.GrantExpiresAt
	default:
		return nil
	}
	if cutoff == "" {
		return nil
	}
	exp, err := time.Parse(time.RFC3339Nano, cutoff)
	if err != nil || now.Before(exp) {
		return nil
	}
	expiredAt := now.Format(time.RFC3339Nano)
	return r.breakglassStore.Transition(ctx, item.RequestID, item.Status, store.StatusExpired, map[string]interface{}{
		"expiredAt": expiredAt,
	})
}

func breakglassRequestIDFromSAName(name string) string {
	const prefix = "zoa-bg-"
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	return ""
}

func shouldOrphanBreakglassKube(req *store.BreakglassRequest, targetCluster string) bool {
	if req == nil {
		return true
	}
	if req.TargetCluster != targetCluster {
		return false
	}
	switch req.Status {
	case store.StatusPending, store.StatusActive:
		return false
	default:
		return true
	}
}
