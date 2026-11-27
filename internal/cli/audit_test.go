package cli

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

func TestListAudit_WhenEntriesExist_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		listAuditFn: func(_ context.Context, query url.Values) (*client.AuditList, error) {
			if query.Get("limit") != "50" {
				t.Errorf("expected limit=50, got %q", query.Get("limit"))
			}
			return &client.AuditList{
				Items: []client.AuditEntry{
					{
						Timestamp:     "2024-06-01T10:00:00Z",
						Method:        "POST",
						Path:          "/api/v0/trusted-actions/get_pods/run",
						StatusCode:    200,
						Operator:      "arn:aws:iam::123456:user/slopezma",
						Action:        "get_pods",
						TargetCluster: "mc-useast1-1",
						Reason:        "TEST-12345",
						ApprovalState: "not_required",
						ExecutionID:   "exec-abc",
					},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &auditOptions{limit: 50}
	err := listAudit(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAudit_WhenNoEntriesExist_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		listAuditFn: func(_ context.Context, _ url.Values) (*client.AuditList, error) {
			return &client.AuditList{Items: []client.AuditEntry{}}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &auditOptions{limit: 50}
	err := listAudit(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAudit_WhenFiltersProvided_ItShouldPassQueryParameters(t *testing.T) {
	mock := &mockClient{
		listAuditFn: func(_ context.Context, query url.Values) (*client.AuditList, error) {
			if query.Get("target") != "mc-useast1-1" {
				t.Errorf("expected target=mc-useast1-1, got %q", query.Get("target"))
			}
			if query.Get("action") != "delete_pod" {
				t.Errorf("expected action=delete_pod, got %q", query.Get("action"))
			}
			if query.Get("method") != "POST" {
				t.Errorf("expected method=POST, got %q", query.Get("method"))
			}
			if query.Get("since") != "24h" {
				t.Errorf("expected since=24h, got %q", query.Get("since"))
			}
			if query.Get("operator") != "slopezma" {
				t.Errorf("expected operator=slopezma, got %q", query.Get("operator"))
			}
			if query.Get("approval_state") != "peer" {
				t.Errorf("expected approval_state=peer, got %q", query.Get("approval_state"))
			}
			return &client.AuditList{Items: []client.AuditEntry{}}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &auditOptions{
		target:   "mc-useast1-1",
		action:   "delete_pod",
		method:   "POST",
		since:    "24h",
		operator: "slopezma",
		approval: "peer",
		limit:    50,
	}
	err := listAudit(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAudit_WhenJSONFormat_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		listAuditFn: func(_ context.Context, _ url.Values) (*client.AuditList, error) {
			return &client.AuditList{
				Items: []client.AuditEntry{
					{Timestamp: "2024-06-01T10:00:00Z", Method: "GET", StatusCode: 200},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatJSON
	opts := &auditOptions{limit: 50}
	err := listAudit(context.Background(), global, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAudit_WhenWideFormat_ItShouldIncludeSignerARNAndSessionID(t *testing.T) {
	signer := "arn:aws:sts::123456:assumed-role/task-role/" + "6e8699e3938a4bcd1234567890abcdef"
	mock := &mockClient{
		listAuditFn: func(_ context.Context, _ url.Values) (*client.AuditList, error) {
			return &client.AuditList{
				Items: []client.AuditEntry{
					{
						Timestamp:  "2024-06-01T10:00:00Z",
						Method:     "POST",
						StatusCode: 200,
						Operator:   "slopezma",
						SignerARN:  signer,
						SessionID:  "sess-abc",
						Action:     "get_pods",
					},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatWide
	opts := &auditOptions{limit: 50}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = listAudit(context.Background(), global, opts)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"SIGNER_ARN", "SESSION_ID", signer, "sess-abc", "slopezma"} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, s)
		}
	}
}

func TestListAudit_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		listAuditFn: func(_ context.Context, _ url.Values) (*client.AuditList, error) {
			return nil, &client.APIError{Code: "internal_error", Reason: "database unavailable"}
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &auditOptions{limit: 50}
	err := listAudit(context.Background(), global, opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "database unavailable" {
		t.Errorf("expected 'database unavailable', got %q", err.Error())
	}
}

func TestListAudit_WhenTableOutput_ItShouldShowRouteFamiliesInPathColumn(t *testing.T) {
	mock := &mockClient{
		listAuditFn: func(_ context.Context, _ url.Values) (*client.AuditList, error) {
			return &client.AuditList{
				Items: []client.AuditEntry{
					{
						Timestamp:  "2024-06-01T10:00:00Z",
						Method:     "GET",
						Path:       "/api/v0/audit",
						StatusCode: 200,
						Operator:   "slopezma",
						Action:     "",
					},
					{
						Timestamp:  "2024-06-01T10:01:00Z",
						Method:     "POST",
						Path:       "/api/v0/sessions/start",
						StatusCode: 200,
						Operator:   "slopezma",
						Action:     "session_start",
					},
					{
						Timestamp:  "2024-06-01T10:02:00Z",
						Method:     "POST",
						Path:       "/api/v0/trusted-actions/list_eks_clusters/run",
						StatusCode: 202,
						Operator:   "slopezma",
						Action:     "list_eks_clusters",
					},
				},
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	opts := &auditOptions{limit: 50}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = listAudit(context.Background(), global, opts)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"audit", "sessions/start", "trusted-actions/list_eks_clusters/run"} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected PATH column to contain %q, got:\n%s", want, s)
		}
	}
	if strings.Contains(s, "/api/v0/") {
		t.Fatalf("table output should not contain /api/v0/, got:\n%s", s)
	}
}

func TestFormatAuditPathForTable_WhenV0Paths_ItShouldStripPrefixOnly(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/api/v0/trusted-actions/list_eks_clusters/run", "trusted-actions/list_eks_clusters/run"},
		{"/api/v0/audit", "audit"},
		{"/api/v0/sessions/start", "sessions/start"},
		{"/api/v0/sessions/join/fcd8cbfb-961d-4ff9-adab-b566c0fdf7fb", "sessions/join"},
		{"/api/v0/breakglass/aws_elevated/activate", "breakglass/aws_elevated/activate"},
		{"/api/v0/breakglass/requests/3ae9b650-7aaf-42be-b9aa-3f45f2f01439/activate", "breakglass/requests/activate"},
		{"/api/v0/approve/eph-regional%2F3ae9b650-7aaf-42be-b9aa-3f45f2f01439", "approve"},
		{"/health", "/health"},
	}
	for _, tc := range tests {
		if got := formatAuditPathForTable(tc.in); got != tc.want {
			t.Errorf("formatAuditPathForTable(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
