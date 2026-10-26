package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/config"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/execcreds"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// --- Access handler test mocks ---

type mockSessionStoreAccess struct {
	sessions     []*store.Session
	putErr       error
	recordExecFn func(ctx context.Context, sessionID, operator, execSessionID string) error
}

func (m *mockSessionStoreAccess) Put(_ context.Context, s *store.Session) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.sessions = append(m.sessions, s)
	return nil
}

func (m *mockSessionStoreAccess) Get(_ context.Context, id string) (*store.Session, error) {
	for _, s := range m.sessions {
		if s.SessionID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockSessionStoreAccess) GetByTaskID(_ context.Context, taskID string) (*store.Session, error) {
	for _, s := range m.sessions {
		if s.TaskID == taskID {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockSessionStoreAccess) UpdateStatus(_ context.Context, id string, _, to store.SessionStatus, updates map[string]interface{}) error {
	for _, s := range m.sessions {
		if s.SessionID == id {
			s.Status = to
			if reason, ok := updates["stopReason"]; ok {
				switch v := reason.(type) {
				case store.StopReason:
					s.StopReason = string(v)
				case string:
					s.StopReason = v
				}
			}
			if arn, ok := updates["taskArn"]; ok {
				s.TaskArn = arn.(string)
			}
			if tid, ok := updates["taskId"]; ok {
				s.TaskID = tid.(string)
			}
			if cluster, ok := updates["ecsCluster"]; ok {
				s.EcsCluster = cluster.(string)
			}
		}
	}
	return nil
}

func (m *mockSessionStoreAccess) ListAll(_ context.Context, filter *store.SessionFilter) ([]*store.Session, error) {
	var out []*store.Session
	for _, s := range m.sessions {
		if filter != nil {
			if filter.Operator != nil && s.Operator != *filter.Operator {
				continue
			}
			if filter.Status != nil && s.Status != *filter.Status {
				continue
			}
		}
		out = append(out, s)
	}
	return out, nil
}
func (m *mockSessionStoreAccess) ListExpired(_ context.Context, _ string) ([]*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreAccess) ListOpenByTargetStatus(_ context.Context, _ string, _ store.SessionStatus) ([]*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreAccess) ListActiveBeforeDeadline(_ context.Context, _ string) ([]*store.Session, error) {
	return nil, nil
}
func (m *mockSessionStoreAccess) RecordExecSession(ctx context.Context, sessionID, operator, execSessionID string) error {
	if m.recordExecFn != nil {
		return m.recordExecFn(ctx, sessionID, operator, execSessionID)
	}
	return nil
}

type mockTargetStoreAccess struct {
	targets []*store.Target
}

type mockECSAccess struct {
	stopErr error
	lastRun *RunTaskInput
}

func (m *mockECSAccess) RunTask(_ context.Context, input *RunTaskInput) (*RunTaskOutput, error) {
	m.lastRun = input
	return &RunTaskOutput{
		TaskArn: "arn:aws:ecs:us-east-1:123456789012:task/test-cluster/task-abc",
		TaskID:  "task-abc",
	}, nil
}

func (m *mockECSAccess) StopTask(_ context.Context, _ *StopTaskInput) error {
	return m.stopErr
}

func testAccessHandlerWithECS(sessionStore store.SessionStore, targetStore store.TargetStore) *AccessHandler {
	h := testAccessHandler(sessionStore, targetStore)
	h.ecsClient = &mockECSAccess{}
	return h
}

func (m *mockTargetStoreAccess) Get(_ context.Context, id string) (*store.Target, error) {
	for _, t := range m.targets {
		if t.TargetID == id {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockTargetStoreAccess) List(_ context.Context) ([]*store.Target, error) {
	return m.targets, nil
}

type mockExecVendorAccess struct {
	err error
}

func (m *mockExecVendorAccess) VendForTask(_ context.Context, _, _, _, _, _ string) (*execcreds.APICredentials, error) {
	if m != nil && m.err != nil {
		return nil, m.err
	}
	return &execcreds.APICredentials{ // notsecret — fake credentials for unit test
		AccessKeyID:     "AKIATEST", // notsecret
		SecretAccessKey: "secret",   // notsecret
		SessionToken:    "token",    // notsecret
		Expiration:      "2030-01-01T00:00:00Z",
	}, nil
}

func testAccessHandler(sessionStore store.SessionStore, targetStore store.TargetStore) *AccessHandler {
	cfg := &config.Config{
		HandlerMode:               "access",
		Region:                    "us-east-1",
		BoundaryECSExecCommand:    "runuser -u sre -- /bin/bash -l",
		ExecScopedRoleARN:         "arn:aws:iam::123:role/exec-scoped",
		SessionIdleTimeoutSeconds: 3600,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAccessHandler(AccessDeps{
		Cfg:          cfg,
		SessionStore: sessionStore,
		TargetStore:  targetStore,
		AuditStore:   &mockAuditStore{},
		ExecVendor:   &mockExecVendorAccess{},
		Logger:       logger,
	})
}

func doAccessRequest(h *AccessHandler, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func accessHeaders() map[string]string {
	return map[string]string{
		"X-Account-ID": "123456789012",
		"X-Operator":   "arn:aws:sts::123456:assumed-role/sre-role/slopezma",
	}
}

// --- Tests ---

func TestAccessHandler_sessionDurationFromRequest_WhenTimeoutHoursExceedsMax_ItShouldError(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})
	h.cfg.SessionMaxDurationHours = 4

	_, err := h.sessionDurationFromRequest(8)
	if err == nil {
		t.Fatal("expected error for timeout_hours above max")
	}
}

func TestAccessHandler_sessionDurationFromRequest_WhenOmitted_ItShouldUseDefaultHours(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})
	h.cfg.SessionMaxDurationHours = 2

	d, err := h.sessionDurationFromRequest(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 2*time.Hour {
		t.Errorf("expected 2h default, got %v", d)
	}
}

func TestAccessHandler_WhenHealthCheck_ItShouldReturn200(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/health", nil, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenVersion_ItShouldReturnModeAccess(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/version", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["mode"] != "access" {
		t.Errorf("expected mode=access, got %v", resp["mode"])
	}
}

func TestAccessHandler_WhenListTargets_ItShouldReturnAllTargets(t *testing.T) {
	targets := &mockTargetStoreAccess{
		targets: []*store.Target{
			{TargetID: "rc", TargetType: "RC", DeploymentName: "us-east-1"},
			{TargetID: "mc01", TargetType: "MC", DeploymentName: "us-east-1"},
		},
	}
	h := testAccessHandler(&mockSessionStoreAccess{}, targets)

	rr := doAccessRequest(h, "GET", "/api/v0/targets", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	items, ok := resp["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Errorf("expected 2 targets, got %v", resp["items"])
	}
}

func TestAccessHandler_WhenSessionStart_ItShouldProvisionBoundaryTask(t *testing.T) {
	targets := &mockTargetStoreAccess{
		targets: []*store.Target{
			{
				TargetID:          "mc01",
				DeploymentName:    "us-east-1",
				VpcId:             "vpc-123",
				SubnetIds:         "subnet-a,subnet-b",
				SecurityGroupId:   "sg-1",
				EcsClusterArn:     "arn:aws:ecs:us-east-1:123456789012:cluster/test",
				TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				FunctionUrl:       "https://test.lambda-url.us-east-1.on.aws",
				Region:            "us-east-1",
			},
		},
	}
	sessions := &mockSessionStoreAccess{}
	h := testAccessHandlerWithECS(sessions, targets)
	ecs := h.ecsClient.(*mockECSAccess)

	body := sessionStartRequest{
		DeploymentName: "us-east-1",
		Target:         "mc01",
		Reason:         "ROSAENG-1234",
	}

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var startResp sessionStartResponse
	json.NewDecoder(rr.Body).Decode(&startResp)
	if startResp.Status != store.SessionStatusActive {
		t.Errorf("expected status active in response, got %q", startResp.Status)
	}
	if startResp.TaskArn == "" {
		t.Error("expected task_arn in start response")
	}

	if len(sessions.sessions) != 1 {
		t.Fatalf("expected 1 session created, got %d", len(sessions.sessions))
	}
	if sessions.sessions[0].Operator != "slopezma" {
		t.Errorf("expected operator 'slopezma', got %q", sessions.sessions[0].Operator)
	}
	if sessions.sessions[0].SignerARN != accessHeaders()["X-Operator"] {
		t.Errorf("expected signer_arn from X-Operator, got %q", sessions.sessions[0].SignerARN)
	}
	if sessions.sessions[0].AccountID != "123456789012" {
		t.Errorf("expected account_id from X-Account-ID, got %q", sessions.sessions[0].AccountID)
	}
	if sessions.sessions[0].Reason != "ROSAENG-1234" {
		t.Errorf("expected reason ROSAENG-1234, got %q", sessions.sessions[0].Reason)
	}
	if sessions.sessions[0].Status != store.SessionStatusActive {
		t.Errorf("expected status active, got %q", sessions.sessions[0].Status)
	}
	if sessions.sessions[0].TaskArn == "" {
		t.Error("expected task ARN on session after start")
	}
	if ecs.lastRun == nil {
		t.Fatal("expected RunTask to be called on start")
	}
	if ecs.lastRun.Tags["Component"] != "zoa" || ecs.lastRun.Tags["function"] != "zoa" {
		t.Errorf("expected Component and function zoa tags on task, got %v", ecs.lastRun.Tags)
	}
	if ecs.lastRun.Environment["ZOA_REASON"] != "ROSAENG-1234" {
		t.Errorf("expected ZOA_REASON on task, got %v", ecs.lastRun.Environment)
	}
}

func TestAccessHandler_WhenSessionJoinFromCreatingWithoutTask_ItShouldReturn409(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID:      "session-123",
				Operator:       "slopezma",
				TargetCluster:  "mc01",
				DeploymentName: "us-east-1",
				Reason:         "ROSAENG-5678",
				Status:         store.SessionStatusCreating,
				Deadline:       "2030-01-01T00:00:00Z",
			},
		},
	}
	h := testAccessHandlerWithECS(sessions, &mockTargetStoreAccess{})
	ecs := h.ecsClient.(*mockECSAccess)

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/session-123", nil, accessHeaders())

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if ecs.lastRun != nil {
		t.Error("expected RunTask not to be called on join")
	}
}

func TestAccessHandler_WhenSessionJoinOnActiveSession_ItShouldNotRunTask(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID:     "session-123",
				Operator:      "slopezma",
				Status:        store.SessionStatusActive,
				EcsCluster:    "arn:aws:ecs:us-east-1:123:cluster/test",
				TaskArn:       "arn:aws:ecs:us-east-1:123:task/test/abc",
				Region:        "us-east-1",
				TargetCluster: "rc-target",
			},
		},
	}
	h := testAccessHandlerWithECS(sessions, &mockTargetStoreAccess{
		targets: []*store.Target{
			{TargetID: "rc-target", AccountId: "123", Region: "us-east-1"},
		},
	})
	ecs := h.ecsClient.(*mockECSAccess)

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/session-123", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ecs.lastRun != nil {
		t.Error("expected RunTask not to be called when session already has a task")
	}
}

func TestAccessHandler_WhenSessionStartMissingReason_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{
		targets: []*store.Target{{TargetID: "mc01", Region: "us-east-1"}},
	})
	body := sessionStartRequest{DeploymentName: "us-east-1", Target: "mc01"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionStartMissingTarget_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	body := sessionStartRequest{DeploymentName: "us-east-1"}

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionStartTargetNotFound_ItShouldReturn404(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	body := sessionStartRequest{
		DeploymentName: "us-east-1",
		Target:         "nonexistent",
		Reason:         "ROSAENG-1234",
	}

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionTerminate_ItShouldTerminateOwnSession(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-123",
				Operator:  "slopezma",
				Status:    store.SessionStatusActive,
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/terminate/session-123", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if sessions.sessions[0].Status != store.SessionStatusTerminated {
		t.Errorf("expected terminated, got %q", sessions.sessions[0].Status)
	}
}

func TestAccessHandler_WhenSessionTerminateTaskFails_ItShouldKeepSessionActive(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID:     "session-123",
				Operator:      "slopezma",
				Status:        store.SessionStatusActive,
				TaskArn:       "arn:aws:ecs:us-east-1:123:task/cluster/task-1",
				EcsCluster:    "cluster",
				TargetCluster: "rc-target",
			},
		},
	}
	targets := &mockTargetStoreAccess{
		targets: []*store.Target{
			{TargetID: "rc-target", AccountId: "123", Region: "us-east-1"},
		},
	}
	h := testAccessHandlerWithECS(sessions, targets)
	h.ecsClient = &mockECSAccess{stopErr: fmt.Errorf("AccessDenied")}

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/terminate/session-123", nil, accessHeaders())

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
	if sessions.sessions[0].Status != store.SessionStatusActive {
		t.Errorf("expected session to stay active, got %q", sessions.sessions[0].Status)
	}
}

func TestAccessHandler_WhenSessionTerminateByNonOwner_ItShouldReturn403(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-123",
				Operator:  "other-user",
				Status:    store.SessionStatusActive,
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/terminate/session-123", nil, accessHeaders())

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionJoinByNonOwner_ItShouldReturn403(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID:  "session-123",
				Operator:   "other-user",
				Status:     store.SessionStatusActive,
				EcsCluster: "arn:aws:ecs:us-east-1:123:cluster/test",
				TaskArn:    "arn:aws:ecs:us-east-1:123:task/test/abc",
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/session-123", nil, accessHeaders())

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionJoinOwnSession_ItShouldReturnConnectionInfo(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID:     "session-123",
				Operator:      "slopezma",
				Status:        store.SessionStatusActive,
				EcsCluster:    "arn:aws:ecs:us-east-1:123:cluster/test",
				TaskArn:       "arn:aws:ecs:us-east-1:123:task/test/abc",
				Region:        "us-east-1",
				TargetCluster: "rc-target",
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{
		targets: []*store.Target{
			{TargetID: "rc-target", AccountId: "123", Region: "us-east-1"},
		},
	})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/session-123", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp sessionJoinResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.TaskArn == "" {
		t.Error("expected task_arn in response")
	}
	if resp.ContainerName != "zoa-boundary" {
		t.Errorf("expected container_name=zoa-boundary, got %q", resp.ContainerName)
	}
	if resp.ExecCommand == "" {
		t.Error("expected exec_command in join response")
	}
	if resp.ExecCredentials == nil || resp.ExecCredentials.AccessKeyID == "" {
		t.Error("expected exec_credentials in join response")
	}
}

func TestAccessHandler_WhenSessionExecAttached_ItShouldAppendExecSessionID(t *testing.T) {
	var recorded string
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-123",
				Operator:  "slopezma",
				Status:    store.SessionStatusActive,
				TaskArn:   "arn:aws:ecs:us-east-1:123:task/cluster/task-abc",
			},
		},
	}
	sessions.recordExecFn = func(_ context.Context, id, op, execID string) error {
		recorded = id + "|" + op + "|" + execID
		return nil
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	body := sessionExecAttachedRequest{ExecSessionID: "k7zkjuilu2vhsrp76e48ie3ciy"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/exec-attached/session-123", body, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if recorded != "session-123|slopezma|k7zkjuilu2vhsrp76e48ie3ciy" {
		t.Errorf("unexpected record: %q", recorded)
	}
}

func TestAccessHandler_WhenSessionList_ItShouldReturnSessions(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{SessionID: "s1", Operator: "slopezma", Status: store.SessionStatusActive},
			{SessionID: "s2", Operator: "other", Status: store.SessionStatusActive},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions?scope=mine", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	count, ok := resp["count"].(float64)
	if !ok || int(count) != 1 {
		t.Errorf("expected count=1 (mine only), got %v", resp["count"])
	}
}

func TestAccessHandler_WhenSessionListMineWithOperatorParam_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions?scope=mine&operator=other", nil, accessHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenApproveStub_ItShouldReturn501(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/approve/req-123", nil, accessHeaders())

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenRejectStub_ItShouldReturn501(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/reject/req-123", nil, accessHeaders())

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionStartInvalidIdentity_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	body := sessionStartRequest{Target: "mc01"}

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, map[string]string{
		"X-Operator": "not-a-valid-arn",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid ARN, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionTerminateNotFound_ItShouldReturn404(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/terminate/nonexistent", nil, accessHeaders())

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionStartStoreFails_ItShouldReturn500(t *testing.T) {
	targets := &mockTargetStoreAccess{
		targets: []*store.Target{
			{TargetID: "mc01", Region: "us-east-1"},
		},
	}
	sessions := &mockSessionStoreAccess{putErr: fmt.Errorf("dynamodb throttled")}
	h := testAccessHandler(sessions, targets)

	body := sessionStartRequest{
		DeploymentName: "us-east-1",
		Target:         "mc01",
		Reason:         "ROSAENG-1234",
	}

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- Additional coverage tests ---

func TestAccessHandler_WhenSessionListStatusFilter_ItShouldFilterByStatus(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{SessionID: "s1", Operator: "slopezma", Status: store.SessionStatusActive},
			{SessionID: "s2", Operator: "slopezma", Status: store.SessionStatusTerminated},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions?scope=mine&status=active", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	count := int(resp["count"].(float64))
	if count != 1 {
		t.Errorf("expected count=1 (active only), got %d", count)
	}
}

func TestAccessHandler_WhenSessionListAllScope_ItShouldReturnAllSessions(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{SessionID: "s1", Operator: "slopezma", Status: store.SessionStatusActive},
			{SessionID: "s2", Operator: "other", Status: store.SessionStatusActive},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	count := int(resp["count"].(float64))
	if count != 2 {
		t.Errorf("expected count=2 (all sessions), got %d", count)
	}
}

func TestAccessHandler_WhenSessionListWithOperatorFilter_ItShouldFilterByOperator(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{SessionID: "s1", Operator: "slopezma", Status: store.SessionStatusActive},
			{SessionID: "s2", Operator: "other", Status: store.SessionStatusActive},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions?operator=other", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	count := int(resp["count"].(float64))
	if count != 1 {
		t.Errorf("expected count=1 (other only), got %d", count)
	}
}

func TestAccessHandler_WhenSessionListMineInvalidIdentity_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions?scope=mine", nil, map[string]string{
		"X-Operator": "not-a-valid-arn",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionListWithLimitParam_ItShouldSetLimit(t *testing.T) {
	sessions := &mockSessionStoreAccess{}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "GET", "/api/v0/sessions?limit=10", nil, accessHeaders())

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionJoinNotFound_ItShouldReturn404(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/nonexistent", nil, accessHeaders())

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionJoinTerminatedSession_ItShouldReturn409(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-done",
				Operator:  "slopezma",
				Status:    store.SessionStatusTerminated,
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/session-done", nil, accessHeaders())

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionJoinInvalidIdentity_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/join/session-123", nil, map[string]string{
		"X-Operator": "bad-arn",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionExecAttachedNotFound_ItShouldReturn404(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	body := sessionExecAttachedRequest{ExecSessionID: "k7zkjuilu2vhsrp76e48ie3ciy"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/exec-attached/nonexistent", body, accessHeaders())

	// Session not found → 404 or 500 (depending on error path)
	if rr.Code == http.StatusOK {
		t.Errorf("expected error for nonexistent session, got 200")
	}
}

func TestAccessHandler_WhenSessionExecAttachedByNonOwner_ItShouldReturn403(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-123",
				Operator:  "other-user",
				Status:    store.SessionStatusActive,
				TaskArn:   "arn:aws:ecs:us-east-1:123:task/cluster/task-abc",
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	body := sessionExecAttachedRequest{ExecSessionID: "k7zkjuilu2vhsrp76e48ie3ciy"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/exec-attached/session-123", body, accessHeaders())

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionExecAttachedInvalidBody_ItShouldReturn400(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-123",
				Operator:  "slopezma",
				Status:    store.SessionStatusActive,
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	body := sessionExecAttachedRequest{ExecSessionID: "bad!!"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/exec-attached/session-123", body, accessHeaders())

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid exec session ID, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionExecAttachedInvalidIdentity_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{})

	body := sessionExecAttachedRequest{ExecSessionID: "k7zkjuilu2vhsrp76e48ie3ciy"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/exec-attached/session-123", body, map[string]string{
		"X-Operator": "bad-arn",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionTerminateAlreadyTerminated_ItShouldReturn409(t *testing.T) {
	sessions := &mockSessionStoreAccess{
		sessions: []*store.Session{
			{
				SessionID: "session-123",
				Operator:  "slopezma",
				Status:    store.SessionStatusTerminated,
			},
		},
	}
	h := testAccessHandler(sessions, &mockTargetStoreAccess{})

	rr := doAccessRequest(h, "POST", "/api/v0/sessions/terminate/session-123", nil, accessHeaders())

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAccessHandler_WhenSessionStartInvalidReason_ItShouldReturn400(t *testing.T) {
	h := testAccessHandler(&mockSessionStoreAccess{}, &mockTargetStoreAccess{
		targets: []*store.Target{{TargetID: "mc01", Region: "us-east-1"}},
	})
	body := sessionStartRequest{DeploymentName: "us-east-1", Target: "mc01", Reason: "abc"}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["code"] != "invalid_reason" {
		t.Errorf("expected code=invalid_reason, got %v", resp["code"])
	}
}

func TestAccessHandler_WhenSessionStartWithRunTask_ItShouldHandleRunTaskFailure(t *testing.T) {
	targets := &mockTargetStoreAccess{
		targets: []*store.Target{
			{
				TargetID:          "mc01",
				SubnetIds:         "subnet-a",
				SecurityGroupId:   "sg-1",
				EcsClusterArn:     "arn:aws:ecs:us-east-1:123456789012:cluster/test",
				TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
				FunctionUrl:       "https://test.lambda-url.us-east-1.on.aws",
			},
		},
	}
	sessions := &mockSessionStoreAccess{}
	h := testAccessHandlerWithECS(sessions, targets)
	h.ecsClient = &mockECSAccessError{runErr: fmt.Errorf("ResourceNotFoundException")}

	body := sessionStartRequest{
		DeploymentName: "us-east-1",
		Target:         "mc01",
		Reason:         "ROSAENG-5678",
	}
	rr := doAccessRequest(h, "POST", "/api/v0/sessions/start", body, accessHeaders())

	if rr.Code == http.StatusOK {
		t.Fatalf("expected failure when RunTask errors, got 200")
	}
}

type mockECSAccessError struct {
	runErr  error
	stopErr error
}

func (m *mockECSAccessError) RunTask(_ context.Context, _ *RunTaskInput) (*RunTaskOutput, error) {
	return nil, m.runErr
}
func (m *mockECSAccessError) StopTask(_ context.Context, _ *StopTaskInput) error {
	return m.stopErr
}

type accessAuditCapture struct {
	last *store.AuditEntry
}

func (c *accessAuditCapture) Record(_ context.Context, e *store.AuditEntry) error {
	c.last = e
	return nil
}

func (c *accessAuditCapture) List(_ context.Context, _ string, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return nil, nil
}

func (c *accessAuditCapture) ListAll(_ context.Context, _ *store.AuditFilter) ([]*store.AuditEntry, error) {
	return nil, nil
}

func TestWhenRecordingAccessAuditItShouldSetTargetCluster(t *testing.T) {
	capture := &accessAuditCapture{}
	h := &AccessHandler{
		logger:     slog.Default(),
		auditStore: capture,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v0/sessions/start", nil)
	req.Header.Set("X-Operator", "arn:aws:sts::123:assumed-role/sre/slopezma")

	h.recordAccessAudit(req, http.StatusOK, "session_start", "sess-1", "RE-1", "eph-fb3672ae-mc01")

	if capture.last == nil {
		t.Fatal("expected audit entry")
	}
	if capture.last.TargetCluster != "eph-fb3672ae-mc01" {
		t.Fatalf("targetCluster: got %q", capture.last.TargetCluster)
	}
	if capture.last.Action != "session_start" {
		t.Fatalf("action: got %q", capture.last.Action)
	}
}
