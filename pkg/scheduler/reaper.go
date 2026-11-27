package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/metrics"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

const (
	// DefaultSessionIdleTimeout is the default idle period after the last exec terminal activity.
	DefaultSessionIdleTimeout = time.Hour
	// DefaultStaleCreatingTimeout is how long a row may stay in creating before the reaper cleans it up.
	DefaultStaleCreatingTimeout = 15 * time.Minute

	sessionPhaseDeadline      = "deadline"
	sessionPhaseStaleCreating = "stale_creating"
	sessionPhaseIdle          = "idle"
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
	sessionStore         store.SessionStore
	ecs                  ECSAPI
	activity             ExecActivityChecker
	idleTimeout          time.Duration
	staleCreatingTimeout time.Duration
	logger               *slog.Logger
	targetCluster        string // per-VPC: only sessions for this cluster (TARGET_CLUSTER)
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

// WithStaleCreatingTimeout sets how long creating rows may linger before reaper cleanup.
func WithStaleCreatingTimeout(d time.Duration) ReaperOption {
	return func(r *Reaper) {
		if d > 0 {
			r.staleCreatingTimeout = d
		}
	}
}

// NewReaper creates a new session reaper. targetCluster is passed to session store
// queries (target-status-index). Production Worker sets TARGET_CLUSTER; tests may use
// mocks with an empty target when the store is not exercised.
func NewReaper(sessionStore store.SessionStore, ecs ECSAPI, logger *slog.Logger, targetCluster string, opts ...ReaperOption) *Reaper {
	r := &Reaper{
		sessionStore:         sessionStore,
		ecs:                  ecs,
		logger:               logger,
		targetCluster:        targetCluster,
		idleTimeout:          DefaultSessionIdleTimeout,
		staleCreatingTimeout: DefaultStaleCreatingTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run enforces deadline, stale creating, and idle policies on open boundary sessions.
func (r *Reaper) Run(ctx context.Context) error {
	start := time.Now()
	var phaseErrors int

	if err := r.runDeadline(ctx); err != nil {
		logWorkerPhaseError(r.logger, err, PillarSessions, sessionPhaseDeadline)
		phaseErrors++
	}
	if err := r.runStaleCreating(ctx); err != nil {
		logWorkerPhaseError(r.logger, err, PillarSessions, sessionPhaseStaleCreating)
		phaseErrors++
	}
	if r.activity != nil && r.idleTimeout > 0 {
		if err := r.runIdle(ctx); err != nil {
			logWorkerPhaseError(r.logger, err, PillarSessions, sessionPhaseIdle)
			phaseErrors++
		}
	}

	metrics.EmitReaper(r.targetCluster, time.Since(start).Milliseconds(), phaseErrors)
	if phaseErrors > 0 {
		return fmt.Errorf("reaper completed with %d phase errors", phaseErrors)
	}
	return nil
}

func (r *Reaper) runDeadline(ctx context.Context) error {
	r.logger.Info("reaper: starting expired session scan", "pillar", PillarSessions, "phase", sessionPhaseDeadline)

	expired, err := r.sessionStore.ListExpired(ctx, r.targetCluster)
	if err != nil {
		return fmt.Errorf("reaper: failed to list expired sessions: %w", err)
	}

	if len(expired) == 0 {
		r.logger.Info("reaper: no expired sessions found", "pillar", PillarSessions, "phase", sessionPhaseDeadline)
		return nil
	}

	r.logger.Info("reaper: found expired sessions",
		"pillar", PillarSessions, "phase", sessionPhaseDeadline,
		"count", len(expired), "target_cluster", r.targetCluster)

	batch := &terminationBatch{}
	for _, session := range expired {
		batch.record(session.SessionID, r.terminateSession(ctx, session, store.StopReasonDeadlineReaperStop, sessionPhaseDeadline))
	}
	batch.logOutcome(r.logger, sessionPhaseDeadline)
	return batch.fail(sessionPhaseDeadline)
}

func (r *Reaper) runStaleCreating(ctx context.Context) error {
	if r.staleCreatingTimeout <= 0 {
		return nil
	}
	r.logger.Info("reaper: starting stale creating session scan",
		"pillar", PillarSessions, "phase", sessionPhaseStaleCreating, "stale_timeout", r.staleCreatingTimeout)

	creating, err := r.sessionStore.ListOpenByTargetStatus(ctx, r.targetCluster, store.SessionStatusCreating)
	if err != nil {
		return fmt.Errorf("reaper: failed to list creating sessions: %w", err)
	}

	cutoff := time.Now().Add(-r.staleCreatingTimeout)
	batch := &terminationBatch{}
	for _, session := range creating {
		created, err := parseSessionTimestamp(session.CreatedAt)
		if err != nil {
			r.logger.Warn("reaper: skip stale creating check, invalid createdAt",
				"pillar", PillarSessions, "phase", sessionPhaseStaleCreating,
				"session_id", session.SessionID, "created_at", session.CreatedAt, "error", err)
			continue
		}
		if created.After(cutoff) {
			continue
		}
		batch.record(session.SessionID, r.terminateSession(ctx, session, store.StopReasonStaleCreatingReaper, sessionPhaseStaleCreating))
	}

	if len(batch.attempted) == 0 {
		r.logger.Info("reaper: no stale creating sessions found", "pillar", PillarSessions, "phase", sessionPhaseStaleCreating)
	} else {
		batch.logOutcome(r.logger, sessionPhaseStaleCreating)
	}
	return batch.fail(sessionPhaseStaleCreating)
}

func (r *Reaper) runIdle(ctx context.Context) error {
	r.logger.Info("reaper: starting idle session scan",
		"pillar", PillarSessions, "phase", sessionPhaseIdle, "idle_timeout", r.idleTimeout)

	active, err := r.sessionStore.ListActiveBeforeDeadline(ctx, r.targetCluster)
	if err != nil {
		return fmt.Errorf("reaper: failed to list active sessions: %w", err)
	}

	cutoff := time.Now().Add(-r.idleTimeout)

	batch := &terminationBatch{}
	var errs []error
	for _, session := range active {
		if session.TaskID == "" || session.TargetCluster == "" {
			continue
		}

		lastActivity, found, err := r.activity.LastTerminalActivity(ctx, session.TargetCluster, session.TaskID, session.ExecSessionIDs)
		if err != nil {
			log := r.logger.With("session_id", session.SessionID, "task_id", session.TaskID)
			log.Error("reaper: idle activity check failed",
				"pillar", PillarSessions, "phase", sessionPhaseIdle, "error", err)
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
					"pillar", PillarSessions, "phase", sessionPhaseIdle,
					"session_id", session.SessionID, "created_at", session.CreatedAt, "error", err)
				continue
			}
			if created.After(cutoff) {
				continue
			}
		}

		batch.record(session.SessionID, r.terminateSession(ctx, session, store.StopReasonIdleReaperStop, sessionPhaseIdle))
	}

	if len(batch.attempted) == 0 && len(errs) == 0 {
		r.logger.Info("reaper: no idle sessions found", "pillar", PillarSessions, "phase", sessionPhaseIdle)
	} else if len(batch.attempted) > 0 {
		batch.logOutcome(r.logger, sessionPhaseIdle)
	}

	idleErrs := append(errs, batch.errs...)
	if len(idleErrs) > 0 {
		return fmt.Errorf("reaper: idle: %d error(s): %w", len(idleErrs), errors.Join(idleErrs...))
	}
	return nil
}

// terminationBatch tracks per-session terminate attempts for reaper logging and errors.
type terminationBatch struct {
	attempted []string
	succeeded []string
	errs      []error
}

func (b *terminationBatch) record(sessionID string, err error) {
	b.attempted = append(b.attempted, sessionID)
	if err != nil {
		b.errs = append(b.errs, err)
		return
	}
	b.succeeded = append(b.succeeded, sessionID)
}

func (b *terminationBatch) logOutcome(logger *slog.Logger, phase string) {
	logger.Info("reaper: termination attempts",
		"pillar", PillarSessions, "phase", phase,
		"attempted", len(b.attempted),
		"succeeded", len(b.succeeded),
		"attempted_session_ids", strings.Join(b.attempted, ","),
	)
	if len(b.succeeded) > 0 {
		logger.Info("reaper: sessions terminated",
			"pillar", PillarSessions, "phase", phase,
			"succeeded", len(b.succeeded),
			"session_ids", strings.Join(b.succeeded, ","),
		)
	}
}

func (b *terminationBatch) fail(phase string) error {
	if len(b.errs) == 0 {
		return nil
	}
	return fmt.Errorf("reaper: %s: %d of %d termination attempts failed: %w",
		phase, len(b.errs), len(b.attempted), errors.Join(b.errs...))
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

func (r *Reaper) terminateSession(ctx context.Context, session *store.Session, reason store.StopReason, phase string) error {
	log := r.logger.With(
		"pillar", PillarSessions,
		"phase", phase,
		"session_id", session.SessionID,
		"operator", session.Operator,
		"stop_reason", reason,
	)

	if session.TaskArn != "" && session.EcsCluster != "" && r.ecs != nil {
		log.Info("reaper: stopping ECS task", "task_arn", session.TaskArn)
		if err := r.ecs.StopTask(ctx, session.EcsCluster, session.TaskArn, string(reason)); err != nil {
			log.Error("reaper: failed to stop ECS task", "error", err)
			return fmt.Errorf("stop task %s: %w", session.SessionID, err)
		}
	}

	from := session.Status
	if from != store.SessionStatusActive && from != store.SessionStatusCreating {
		log.Warn("reaper: skip terminate, unexpected session status", "status", from)
		return fmt.Errorf("unexpected status %s for session %s", from, session.SessionID)
	}
	if err := r.sessionStore.UpdateStatus(ctx, session.SessionID,
		from, store.SessionStatusTerminated,
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
