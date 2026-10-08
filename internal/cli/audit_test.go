package cli

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
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
						Reason:        "ROSAENG-1234",
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
