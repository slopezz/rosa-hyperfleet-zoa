package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// BreakglassReconciler expires pending and approved break-glass grants and GCs kube RBAC.
type BreakglassReconciler struct {
	store         store.BreakglassStore
	exec          *executor.Executor
	targetCluster string
	logger        *slog.Logger
}

func NewBreakglassReconciler(s store.BreakglassStore, exec *executor.Executor, targetCluster string, logger *slog.Logger) *BreakglassReconciler {
	return &BreakglassReconciler{store: s, exec: exec, targetCluster: targetCluster, logger: logger}
}

func (r *BreakglassReconciler) Run(ctx context.Context) error {
	now := time.Now()
	for _, status := range []store.Status{store.StatusPendingApproval, store.StatusApproved} {
		items, err := r.store.QueryByTargetAndStatus(ctx, r.targetCluster, status)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := r.expireIfNeeded(ctx, item, now); err != nil {
				r.logger.Error("breakglass expire", "requestId", item.RequestID, "error", err)
			}
		}
	}
	return r.garbageCollect(ctx)
}

func (r *BreakglassReconciler) garbageCollect(ctx context.Context) error {
	if r.exec == nil {
		return nil
	}
	items, err := r.store.QueryByTargetAndStatus(ctx, r.targetCluster, store.StatusExpired)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Cleaned {
			continue
		}
		if def, ok := breakglass.Get(item.Scope); ok && def.Metadata.Plane == breakglass.PlaneKube {
			r.exec.CleanupBreakglass(ctx, item.RequestID)
		}
		if err := r.store.MarkCleaned(ctx, item.RequestID); err != nil {
			r.logger.Error("breakglass mark cleaned", "requestId", item.RequestID, "error", err)
		}
	}
	return nil
}

func (r *BreakglassReconciler) expireIfNeeded(ctx context.Context, item *store.BreakglassRequest, now time.Time) error {
	var cutoff string
	switch item.Status {
	case store.StatusPendingApproval:
		cutoff = item.RequestExpiresAt
	case store.StatusApproved:
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
	return r.store.Transition(ctx, item.RequestID, item.Status, store.StatusExpired, map[string]interface{}{
		"grantExpiresAt": cutoff,
	})
}
