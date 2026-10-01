package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

const (
	// SessionMaxDuration is the maximum session duration before the reaper terminates it.
	SessionMaxDuration = 4 * time.Hour
)

// ECSAPI abstracts ECS operations for the reaper.
type ECSAPI interface {
	StopTask(ctx context.Context, cluster, taskArn, reason string) error
}

// Reaper enforces session deadlines by terminating expired boundary sessions.
type Reaper struct {
	sessionStore  store.SessionStore
	ecs           ECSAPI
	logger        *slog.Logger
	targetCluster string // per-VPC: only sessions for this cluster (TARGET_CLUSTER)
}

// NewReaper creates a new session reaper. targetCluster scopes StopTask to this
// VPC's boundary sessions (empty disables filtering — tests only).
func NewReaper(sessionStore store.SessionStore, ecs ECSAPI, logger *slog.Logger, targetCluster string) *Reaper {
	return &Reaper{
		sessionStore:  sessionStore,
		ecs:           ecs,
		logger:        logger,
		targetCluster: targetCluster,
	}
}

// Run queries for expired sessions and terminates them.
func (r *Reaper) Run(ctx context.Context) error {
	r.logger.Info("reaper: starting expired session scan")

	expired, err := r.sessionStore.ListExpired(ctx)
	if err != nil {
		return fmt.Errorf("reaper: failed to list expired sessions: %w", err)
	}

	if r.targetCluster != "" {
		filtered := expired[:0]
		for _, session := range expired {
			if session.TargetCluster == r.targetCluster {
				filtered = append(filtered, session)
			}
		}
		expired = filtered
	}

	if len(expired) == 0 {
		r.logger.Info("reaper: no expired sessions found")
		return nil
	}

	r.logger.Info("reaper: found expired sessions", "count", len(expired), "target_cluster", r.targetCluster)

	var errs []error
	for _, session := range expired {
		log := r.logger.With("session_id", session.SessionID, "operator", session.Operator)

		// Stop the ECS task if it's still running.
		if session.TaskArn != "" && session.EcsCluster != "" && r.ecs != nil {
			log.Info("reaper: stopping ECS task", "task_arn", session.TaskArn)
			if err := r.ecs.StopTask(ctx, session.EcsCluster, session.TaskArn, store.StopReasonDeadlineReaperStop); err != nil {
				log.Error("reaper: failed to stop ECS task", "error", err)
				errs = append(errs, fmt.Errorf("stop task %s: %w", session.SessionID, err))
				continue
			}
		}

		// Transition to terminated.
		if err := r.sessionStore.UpdateStatus(ctx, session.SessionID,
			store.SessionStatusActive, store.SessionStatusTerminated,
			map[string]interface{}{
				"stopReason": store.StopReasonDeadlineReaperStop,
			},
		); err != nil {
			log.Error("reaper: failed to update session status", "error", err)
			errs = append(errs, fmt.Errorf("update status %s: %w", session.SessionID, err))
			continue
		}

		log.Info("reaper: terminated expired session")
	}

	if len(errs) > 0 {
		return fmt.Errorf("reaper: %d of %d sessions failed to terminate: %v", len(errs), len(expired), errs[0])
	}

	return nil
}
