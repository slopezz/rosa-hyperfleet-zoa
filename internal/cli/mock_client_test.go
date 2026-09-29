package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

type mockClient struct {
	dispatchFn         func(ctx context.Context, action string, req *client.DispatchRequest) (*client.DispatchResponse, error)
	getExecutionFn     func(ctx context.Context, id string, include string) (*client.Execution, error)
	listExecutionsFn   func(ctx context.Context, query url.Values) (*client.ExecutionList, error)
	getActionFn        func(ctx context.Context, name string) (*client.Action, error)
	listActionsFn      func(ctx context.Context) (*client.ActionList, error)
	listAuditFn        func(ctx context.Context, query url.Values) (*client.AuditList, error)
	serverVersionFn    func(ctx context.Context) (*client.ServerVersionInfo, error)
	rawGetFn           func(ctx context.Context, path string) (*http.Response, error)
	listTargetsFn      func(ctx context.Context) (*client.TargetList, error)
	listTargetsByDepFn func(ctx context.Context, deployment string) (*client.TargetList, error)
	sessionStartFn     func(ctx context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error)
	sessionStopFn      func(ctx context.Context, sessionID string) error
	sessionJoinFn      func(ctx context.Context, sessionID string) (*client.SessionJoinResponse, error)
	listSessionsFn     func(ctx context.Context, query url.Values) (*client.SessionList, error)
}

func (m *mockClient) Dispatch(ctx context.Context, action string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
	if m.dispatchFn != nil {
		return m.dispatchFn(ctx, action, req)
	}
	return nil, fmt.Errorf("Dispatch not mocked")
}

func (m *mockClient) GetExecution(ctx context.Context, id string, include string) (*client.Execution, error) {
	if m.getExecutionFn != nil {
		return m.getExecutionFn(ctx, id, include)
	}
	return nil, fmt.Errorf("GetExecution not mocked")
}

func (m *mockClient) ListExecutions(ctx context.Context, query url.Values) (*client.ExecutionList, error) {
	if m.listExecutionsFn != nil {
		return m.listExecutionsFn(ctx, query)
	}
	return nil, fmt.Errorf("ListExecutions not mocked")
}

func (m *mockClient) GetAction(ctx context.Context, name string) (*client.Action, error) {
	if m.getActionFn != nil {
		return m.getActionFn(ctx, name)
	}
	return nil, fmt.Errorf("GetAction not mocked")
}

func (m *mockClient) ListActions(ctx context.Context) (*client.ActionList, error) {
	if m.listActionsFn != nil {
		return m.listActionsFn(ctx)
	}
	return nil, fmt.Errorf("ListActions not mocked")
}

func (m *mockClient) ListAudit(ctx context.Context, query url.Values) (*client.AuditList, error) {
	if m.listAuditFn != nil {
		return m.listAuditFn(ctx, query)
	}
	return nil, fmt.Errorf("ListAudit not mocked")
}

func (m *mockClient) ServerVersion(ctx context.Context) (*client.ServerVersionInfo, error) {
	if m.serverVersionFn != nil {
		return m.serverVersionFn(ctx)
	}
	return nil, fmt.Errorf("ServerVersion not mocked")
}

func (m *mockClient) RawGet(ctx context.Context, path string) (*http.Response, error) {
	if m.rawGetFn != nil {
		return m.rawGetFn(ctx, path)
	}
	return nil, fmt.Errorf("RawGet not mocked")
}

func (m *mockClient) ListTargets(ctx context.Context) (*client.TargetList, error) {
	if m.listTargetsFn != nil {
		return m.listTargetsFn(ctx)
	}
	return nil, fmt.Errorf("ListTargets not mocked")
}

func (m *mockClient) ListTargetsByDeployment(ctx context.Context, deployment string) (*client.TargetList, error) {
	if m.listTargetsByDepFn != nil {
		return m.listTargetsByDepFn(ctx, deployment)
	}
	return nil, fmt.Errorf("ListTargetsByDeployment not mocked")
}

func (m *mockClient) SessionStart(ctx context.Context, req *client.SessionStartRequest) (*client.SessionStartResponse, error) {
	if m.sessionStartFn != nil {
		return m.sessionStartFn(ctx, req)
	}
	return nil, fmt.Errorf("SessionStart not mocked")
}

func (m *mockClient) SessionStop(ctx context.Context, sessionID string) error {
	if m.sessionStopFn != nil {
		return m.sessionStopFn(ctx, sessionID)
	}
	return fmt.Errorf("SessionStop not mocked")
}

func (m *mockClient) SessionJoin(ctx context.Context, sessionID string) (*client.SessionJoinResponse, error) {
	if m.sessionJoinFn != nil {
		return m.sessionJoinFn(ctx, sessionID)
	}
	return nil, fmt.Errorf("SessionJoin not mocked")
}

func (m *mockClient) ListSessions(ctx context.Context, query url.Values) (*client.SessionList, error) {
	if m.listSessionsFn != nil {
		return m.listSessionsFn(ctx, query)
	}
	return nil, fmt.Errorf("ListSessions not mocked")
}

func newMockGlobalOpts(mock *mockClient) *GlobalOptions {
	return &GlobalOptions{
		APIURL: "https://test.lambda-url.us-east-1.on.aws",
		ClientFactory: func(_ *GlobalOptions) (APIClient, error) {
			return mock, nil
		},
	}
}
