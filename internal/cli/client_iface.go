package cli

import (
	"context"
	"net/http"
	"net/url"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

// APIClient defines the operations the CLI needs from the ZOA API.
// Extracted as an interface to enable testing without real AWS credentials.
type APIClient interface {
	Dispatch(ctx context.Context, action string, req *client.DispatchRequest) (*client.DispatchResponse, error)
	GetExecution(ctx context.Context, id string, include string) (*client.Execution, error)
	ListExecutions(ctx context.Context, query url.Values) (*client.ExecutionList, error)
	GetAction(ctx context.Context, name string) (*client.Action, error)
	ListActions(ctx context.Context) (*client.ActionList, error)
	ListAudit(ctx context.Context, query url.Values) (*client.AuditList, error)
	ServerVersion(ctx context.Context) (*client.ServerVersionInfo, error)
	RawGet(ctx context.Context, path string) (*http.Response, error)

	// Access / Boundary methods
	ListTargets(ctx context.Context) (*client.TargetList, error)
	ListTargetsByDeployment(ctx context.Context, deployment string) (*client.TargetList, error)
	SessionStart(ctx context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error)
	SessionTerminate(ctx context.Context, sessionID string) error
	SessionJoin(ctx context.Context, sessionID string) (*client.SessionJoinResponse, error)
	SessionExecAttached(ctx context.Context, sessionID, execSessionID string) error
	ListSessions(ctx context.Context, query url.Values) (*client.SessionList, error)
}

// Verify *client.Client satisfies APIClient at compile time.
var _ APIClient = (*client.Client)(nil)
