package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

const (
	// DefaultSessionIdleTimeout is the default idle period after the last exec terminal activity.
	DefaultSessionIdleTimeout = time.Hour
)

// ECSAPI abstracts ECS operations for the reaper.
type ECSAPI interface {
	StopTask(ctx context.Context, cluster, taskArn, reason string) error
}

// ExecActivityChecker reports the latest ECS Exec terminal activity for a boundary task.
type ExecActivityChecker interface {
	LastTerminalActivity(ctx context.Context, targetCluster, taskID string, dynamoExecIDs []string) (time.Time, bool, error)
}

// Reaper enforces session deadlines and idle timeouts by terminating boundary sessions.
type Reaper struct {
	sessionStore  store.SessionStore
	ecs           ECSAPI
	activity      ExecActivityChecker
	idleTimeout   time.Duration
	logger        *slog.Logger
	targetCluster string // per-VPC: only sessions for this cluster (TARGET_CLUSTER)
}

// ReaperOption configures optional reaper behaviour.
type ReaperOption func(*Reaper)

// WithExecActivity enables idle enforcement using SSM + CloudWatch (via ExecActivityChecker).
func WithExecActivity(checker ExecActivityChecker, idleTimeout time.Duration) ReaperOption {
	return func(r *Reaper) {
		r.activity = checker
		if idleTimeout > 0 {
			r.idleTimeout = idleTimeout
		}
	}
}

// NewReaper creates a new session reaper. targetCluster scopes enforcement to this
// VPC's boundary sessions (empty disables filtering — tests only).
func NewReaper(sessionStore store.SessionStore, ecs ECSAPI, logger *slog.Logger, targetCluster string, opts ...ReaperOption) *Reaper {
	r := &Reaper{
		sessionStore:  sessionStore,
		ecs:           ecs,
		logger:        logger,
		targetCluster: targetCluster,
		idleTimeout:   DefaultSessionIdleTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run enforces deadline and idle policies on active boundary sessions.
func (r *Reaper) Run(ctx context.Context) error {
	if err := r.runDeadline(ctx); err != nil {
		return err
	}
	if r.activity != nil && r.idleTimeout > 0 {
		if err := r.runIdle(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reaper) runDeadline(ctx context.Context) error {
	r.logger.Info("reaper: starting expired session scan")

	expired, err := r.sessionStore.ListExpired(ctx)
	if err != nil {
		return fmt.Errorf("reaper: failed to list expired sessions: %w", err)
	}

	expired = r.filterByTargetCluster(expired)

	if len(expired) == 0 {
		r.logger.Info("reaper: no expired sessions found")
		return nil
	}

	r.logger.Info("reaper: found expired sessions", "count", len(expired), "target_cluster", r.targetCluster)

	var errs []error
	for _, session := range expired {
		if err := r.terminateSession(ctx, session, store.StopReasonDeadlineReaperStop); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("reaper: %d of %d deadline terminations failed: %v", len(errs), len(expired), errs[0])
	}
	return nil
}

func (r *Reaper) runIdle(ctx context.Context) error {
	r.logger.Info("reaper: starting idle session scan", "idle_timeout", r.idleTimeout)

	active, err := r.sessionStore.ListActiveBeforeDeadline(ctx)
	if err != nil {
		return fmt.Errorf("reaper: failed to list active sessions: %w", err)
	}

	active = r.filterByTargetCluster(active)
	cutoff := time.Now().Add(-r.idleTimeout)

	var errs []error
	idleCount := 0
	for _, session := range active {
		if session.TaskID == "" || session.TargetCluster == "" {
			continue
		}

		lastActivity, found, err := r.activity.LastTerminalActivity(ctx, session.TargetCluster, session.TaskID, session.ExecSessionIDs)
		if err != nil {
			log := r.logger.With("session_id", session.SessionID, "task_id", session.TaskID)
			log.Error("reaper: idle activity check failed", "error", err)
			errs = append(errs, fmt.Errorf("idle check %s: %w", session.SessionID, err))
			continue
		}

		if found {
			if lastActivity.After(cutoff) {
				continue
			}
		} else {
			// No ECS Exec sessions at AWS for this task (SSM/CW). Reap unused tasks even if
			// the CLI never recorded exec_session_id in DynamoDB.
			created, err := parseSessionTimestamp(session.CreatedAt)
			if err != nil {
				r.logger.Warn("reaper: skip idle check, invalid createdAt",
					"session_id", session.SessionID, "created_at", session.CreatedAt, "error", err)
				continue
			}
			if created.After(cutoff) {
				continue
			}
		}

		idleCount++
		if err := r.terminateSession(ctx, session, store.StopReasonIdleReaperStop); err != nil {
			errs = append(errs, err)
		}
	}

	if idleCount == 0 {
		r.logger.Info("reaper: no idle sessions found")
	} else {
		r.logger.Info("reaper: terminated idle sessions", "count", idleCount)
	}

	if len(errs) > 0 {
		return fmt.Errorf("reaper: %d idle termination errors: %v", len(errs), errs[0])
	}
	return nil
}

func parseSessionTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, value)
}

func (r *Reaper) filterByTargetCluster(sessions []*store.Session) []*store.Session {
	if r.targetCluster == "" {
		return sessions
	}
	filtered := sessions[:0]
	for _, session := range sessions {
		if session.TargetCluster == r.targetCluster {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func (r *Reaper) terminateSession(ctx context.Context, session *store.Session, reason string) error {
	log := r.logger.With("session_id", session.SessionID, "operator", session.Operator, "stop_reason", reason)

	if session.TaskArn != "" && session.EcsCluster != "" && r.ecs != nil {
		log.Info("reaper: stopping ECS task", "task_arn", session.TaskArn)
		if err := r.ecs.StopTask(ctx, session.EcsCluster, session.TaskArn, reason); err != nil {
			log.Error("reaper: failed to stop ECS task", "error", err)
			return fmt.Errorf("stop task %s: %w", session.SessionID, err)
		}
	}

	if err := r.sessionStore.UpdateStatus(ctx, session.SessionID,
		store.SessionStatusActive, store.SessionStatusTerminated,
		map[string]interface{}{
			"stopReason": reason,
		},
	); err != nil {
		log.Error("reaper: failed to update session status", "error", err)
		return fmt.Errorf("update status %s: %w", session.SessionID, err)
	}

	log.Info("reaper: terminated session")
	return nil
}
