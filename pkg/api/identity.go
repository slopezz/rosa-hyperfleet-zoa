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

// ResolveIdentity attributes API requests to a human operator and optional boundary session.
//
// Design: docs/design/boundary-identity-and-storage.md
//
//  1. Identity bridge: RoleSessionName from signer_arn is often the ECS task UUID on
//     boundary TAs. GetByTaskID (GSI task-id-index) → operator + session_id.
//  2. TEMPORARY fallback (laptop zoa run until IAM restricts API invoke to boundary tasks):
//     operator = ExtractSREIdentity(signer_arn); session_id = "".
//
// IAM policies decide who may invoke; this function does not gate on IAM role names.
func ResolveIdentity(ctx context.Context, signerARN string, sessionStore store.SessionStore) (*IdentityResult, error) {
	if signerARN == "" {
		return nil, fmt.Errorf("empty caller ARN")
	}

	sessionName, _, err := ExtractSREIdentity(signerARN)
	if err != nil {
		return nil, err
	}

	if sessionStore != nil && looksLikeECSTaskID(sessionName) {
		session, err := sessionStore.GetByTaskID(ctx, sessionName)
		if err != nil {
			return nil, fmt.Errorf("looking up session for task %q: %w", sessionName, err)
		}
		if session != nil {
			return &IdentityResult{
				Operator:  session.Operator,
				SessionID: session.SessionID,
			}, nil
		}
	}

	// TEMPORARY: laptop TA until IAM restricts API invoke to boundary task role only.
	return &IdentityResult{
		Operator: sessionName,
	}, nil
}

// looksLikeECSTaskID is a performance hint only (ECS task ids are 32-char hex).
func looksLikeECSTaskID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
