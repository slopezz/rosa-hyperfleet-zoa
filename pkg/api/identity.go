package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

// ExtractSREIdentity parses an STS assumed-role ARN to extract the SRE username
// and role name.
//
// ARN format: arn:aws:sts::ACCOUNT:assumed-role/ROLE_NAME/SESSION_NAME
// Example: "arn:aws:sts::123456:assumed-role/sre-role/slopezma" → ("slopezma", "sre-role")
func ExtractSREIdentity(operatorARN string) (username, role string, err error) {
	if operatorARN == "" {
		return "", "", fmt.Errorf("empty operator ARN")
	}

	parts := strings.Split(operatorARN, "/")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid assumed-role ARN format: %q (expected arn:.../role/session)", operatorARN)
	}

	if !strings.Contains(operatorARN, "assumed-role") {
		return "", "", fmt.Errorf("not an assumed-role ARN: %q", operatorARN)
	}

	username = parts[len(parts)-1]
	role = parts[len(parts)-2]
	if username == "" {
		return "", "", fmt.Errorf("empty session name in ARN: %q", operatorARN)
	}
	return username, role, nil
}

// IdentityResult holds the resolved identity fields from a caller ARN.
type IdentityResult struct {
	Operator  string // Human-readable SRE username (e.g., "slopezma")
	SessionID string // Boundary session ID (empty for direct laptop calls)
}

// ResolveIdentity determines the SRE identity from a caller ARN. If the caller
// is a boundary ECS task role, it looks up the session in DynamoDB to find the
// originating SRE and session ID. Otherwise, it extracts the identity directly
// from the ARN (laptop caller, no boundary session).
//
// The boundary task role ARN looks like:
//
//	arn:aws:sts::ACCOUNT:assumed-role/zoa-boundary-task-role/TASK_ID
//
// The task ID is the RoleSessionName set by the ECS agent (tamper-proof — the
// SRE cannot change it). It is used to look up the session in DynamoDB via
// the task-id-index GSI. The session record contains the original SRE's
// username and the session ID. Both are returned, eliminating any reliance
// on client-supplied headers for identity or session linkage.
func ResolveIdentity(ctx context.Context, callerARN string, sessionStore store.SessionStore, boundaryRolePrefix string) (*IdentityResult, error) {
	if callerARN == "" {
		return nil, fmt.Errorf("empty caller ARN")
	}

	sreUsername, role, err := ExtractSREIdentity(callerARN)
	if err != nil {
		return nil, err
	}

	if boundaryRolePrefix == "" {
		boundaryRolePrefix = "zoa-boundary"
	}

	// If the role matches the boundary task role pattern, resolve via DynamoDB.
	// The session name for ECS task roles is the task ID (set by ECS agent,
	// not the SRE — tamper-proof).
	if strings.HasPrefix(role, boundaryRolePrefix) {
		if sessionStore == nil {
			return nil, fmt.Errorf("session store required for boundary task role identity resolution")
		}

		taskID := sreUsername
		session, err := sessionStore.GetByTaskID(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("looking up session for task %q: %w", taskID, err)
		}
		if session == nil {
			return nil, fmt.Errorf("no session found for task %q — cannot resolve SRE identity", taskID)
		}
		return &IdentityResult{
			Operator:  session.Operator,
			SessionID: session.SessionID,
		}, nil
	}

	// Direct caller (laptop) — identity is the session name from the ARN.
	// No boundary session exists.
	return &IdentityResult{
		Operator: sreUsername,
	}, nil
}
