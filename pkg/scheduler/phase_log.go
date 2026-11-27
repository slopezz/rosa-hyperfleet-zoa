package scheduler

import "log/slog"

// Pillar labels for worker phase errors (CloudWatch Logs filter keys).
// TA and break-glass both run garbage_collect / orphan_gc phases — distinguish via pillar.
// Sessions reaper uses pillar=sessions with phases deadline, stale_creating, idle.
const (
	PillarTA         = "ta"
	PillarBreakglass = "breakglass"
	PillarSessions   = "sessions"
)

func logWorkerPhaseError(logger *slog.Logger, err error, pillar, phase string) {
	if logger == nil || err == nil {
		return
	}
	logger.Error("worker phase failed", "error", err, "pillar", pillar, "phase", phase)
}
