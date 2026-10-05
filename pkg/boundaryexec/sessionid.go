package boundaryexec

import (
	"fmt"
	"strings"
)

const execSessionPrefix = "ecs-execute-command-"

// ExecLogGroup returns the CloudWatch log group for ECS Exec transcripts on a target cluster.
func ExecLogGroup(targetCluster string) string {
	return fmt.Sprintf("/ecs/%s/zoa-boundary/ssm-sessions", targetCluster)
}

// LogStreamName returns the CloudWatch log stream name for an exec session id.
func LogStreamName(execSessionID string) string {
	id := NormalizeExecSessionID(execSessionID)
	if strings.HasPrefix(id, execSessionPrefix) {
		return id
	}
	return execSessionPrefix + id
}

// NormalizeExecSessionID strips an optional ecs-execute-command- prefix for comparisons.
func NormalizeExecSessionID(execSessionID string) string {
	id := strings.TrimSpace(execSessionID)
	id = strings.TrimPrefix(id, execSessionPrefix)
	return id
}

// ValidateExecSessionID rejects empty or obviously invalid session ids from clients.
func ValidateExecSessionID(execSessionID string) error {
	id := NormalizeExecSessionID(execSessionID)
	if id == "" {
		return fmt.Errorf("exec_session_id is required")
	}
	if len(id) > 128 {
		return fmt.Errorf("exec_session_id is too long")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return fmt.Errorf("exec_session_id contains invalid character %q", r)
	}
	return nil
}
