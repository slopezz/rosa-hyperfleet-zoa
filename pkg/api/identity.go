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

// ResolveIdentity determines the SRE identity from a caller ARN. If the caller
// is a boundary ECS task role, it looks up the session in DynamoDB to find the
// originating SRE. Otherwise, it extracts the identity directly from the ARN.
//
// The boundary task role ARN looks like:
//
//	arn:aws:sts::ACCOUNT:assumed-role/zoa-boundary-task-role/TASK_ID
//
// The task ID is used to look up the session in DynamoDB, which contains the
// original SRE's username.
func ResolveIdentity(ctx context.Context, callerARN string, sessionStore store.SessionStore, boundaryRolePrefix string) (username string, err error) {
	if callerARN == "" {
		return "", fmt.Errorf("empty caller ARN")
	}

	sreUsername, role, err := ExtractSREIdentity(callerARN)
	if err != nil {
		return "", err
	}

	if boundaryRolePrefix == "" {
		boundaryRolePrefix = "zoa-boundary"
	}

	// If the role matches the boundary task role pattern, resolve via DynamoDB.
	if strings.HasPrefix(role, boundaryRolePrefix) {
		if sessionStore == nil {
			return "", fmt.Errorf("session store required for boundary task role identity resolution")
		}

		// The session name for ECS task roles is the task ID.
		taskID := sreUsername
		session, err := sessionStore.Get(ctx, taskID)
		if err != nil {
			return "", fmt.Errorf("looking up session for task %q: %w", taskID, err)
		}
		if session == nil {
			return "", fmt.Errorf("no session found for task %q — cannot resolve SRE identity", taskID)
		}
		return session.Operator, nil
	}

	return sreUsername, nil
}
