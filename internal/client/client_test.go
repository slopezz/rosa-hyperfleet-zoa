package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type staticCredentials struct{}

func (s staticCredentials) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",                     // notsecret -- AWS docs example credentials
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", // notsecret -- AWS docs example credentials
		SessionToken:    "FwoGZXIvYXdzEBYaDHqa0AP",                  // notsecret -- test fixture, not a real token
		Source:          "test",
	}, nil
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantRegion  string
		wantService string
		wantErr     bool
	}{
		{
			name:        "When URL is API Gateway it should extract region and use execute-api service",
			url:         "https://abc123.execute-api.us-east-1.amazonaws.com/prod",
			wantRegion:  "us-east-1",
			wantService: "execute-api",
		},
		{
			name:        "When URL is API Gateway in eu-west-1 it should extract correctly",
			url:         "https://xyz789.execute-api.eu-west-1.amazonaws.com/prod",
			wantRegion:  "eu-west-1",
			wantService: "execute-api",
		},
		{
			name:        "When URL is Lambda Function URL it should extract region and use lambda service",
			url:         "https://mghe7tpfk3lhxtq66mp5td6epa0iugjm.lambda-url.us-east-1.on.aws/",
			wantRegion:  "us-east-1",
			wantService: "lambda",
		},
		{
			name:        "When URL is Lambda Function URL in eu-central-1 it should extract correctly",
			url:         "https://abcdef1234.lambda-url.eu-central-1.on.aws",
			wantRegion:  "eu-central-1",
			wantService: "lambda",
		},
		{
			name:        "When URL is a custom CNAME it should return error for region auto-detection",
			url:         "https://zoa.internal.example.com/api",
			wantService: "lambda",
			wantErr:     true,
		},
		{
			name:    "When URL is invalid it should return error",
			url:     "://not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, service, err := parseEndpoint(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEndpoint(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if region != tt.wantRegion {
				t.Errorf("parseEndpoint(%q) region = %q, want %q", tt.url, region, tt.wantRegion)
			}
			if service != tt.wantService {
				t.Errorf("parseEndpoint(%q) service = %q, want %q", tt.url, service, tt.wantService)
			}
		})
	}
}

func TestClientDispatch(t *testing.T) {
	t.Run("When API returns success it should parse response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/v0/trusted-actions/get_nodes/run" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DispatchResponse{
				ID:     "exec-123",
				Status: "dispatched",
			})
		}))
		defer server.Close()

		c := &Client{
			baseURL:     server.URL,
			region:      "us-east-1",
			credentials: staticCredentials{},
			httpClient:  http.DefaultClient,
			signer:      nil,
		}

		resp, err := c.Dispatch(context.Background(), "get_nodes", &DispatchRequest{
			Jira: "TEST-123",
		})
		if err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
		if resp.ID != "exec-123" {
			t.Errorf("Dispatch().ID = %q, want %q", resp.ID, "exec-123")
		}
	})

	t.Run("When API returns error it should return APIError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(APIError{
				Code:   "CooldownActive",
				Reason: "write cooldown active (28s remaining)",
			})
		}))
		defer server.Close()

		c := &Client{
			baseURL:     server.URL,
			region:      "us-east-1",
			credentials: staticCredentials{},
			httpClient:  http.DefaultClient,
			signer:      nil,
		}

		_, err := c.Dispatch(context.Background(), "rollout_restart", &DispatchRequest{
			Jira: "TEST-123",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		apiErr, ok := err.(*APIError)
		if !ok {
			t.Fatalf("expected *APIError, got %T: %v", err, err)
		}
		if apiErr.Code != "CooldownActive" {
			t.Errorf("APIError.Code = %q, want %q", apiErr.Code, "CooldownActive")
		}
	})
}

func TestClientGetExecution(t *testing.T) {
	t.Run("When include=output it should pass query parameter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions/runs/exec-456" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.Query().Get("include") != "output" {
				t.Errorf("expected include=output, got %q", r.URL.Query().Get("include"))
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Execution{
				ID:     "exec-456",
				Status: "succeeded",
				Output: "node1 Ready\n",
			})
		}))
		defer server.Close()

		c := &Client{
			baseURL:     server.URL,
			region:      "us-east-1",
			credentials: staticCredentials{},
			httpClient:  http.DefaultClient,
			signer:      nil,
		}

		exec, err := c.GetExecution(context.Background(), "exec-456", "output")
		if err != nil {
			t.Fatalf("GetExecution() error = %v", err)
		}
		if exec.Output.String() != "node1 Ready\n" {
			t.Errorf("GetExecution().Output = %q, want %q", exec.Output, "node1 Ready\n")
		}
	})

	t.Run("When include is empty it should not append query string", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				t.Errorf("expected no query string, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Execution{ID: "exec-no-include", Status: "dispatched"})
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		exec, err := c.GetExecution(context.Background(), "exec-no-include", "")
		if err != nil {
			t.Fatalf("GetExecution() error = %v", err)
		}
		if exec.Status != "dispatched" {
			t.Errorf("expected status dispatched, got %q", exec.Status)
		}
	})
}

func TestClientListExecutions(t *testing.T) {
	t.Run("When query params provided it should pass them", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions/runs" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.Query().Get("status") != "failed" {
				t.Errorf("expected status=failed, got %q", r.URL.Query().Get("status"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ExecutionList{Items: []Execution{{ID: "e1", Status: "failed"}}})
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		q := make(map[string][]string)
		q["status"] = []string{"failed"}
		list, err := c.ListExecutions(context.Background(), q)
		if err != nil {
			t.Fatalf("ListExecutions() error = %v", err)
		}
		if len(list.Items) != 1 {
			t.Errorf("expected 1 item, got %d", len(list.Items))
		}
	})
}

func TestClientGetAction(t *testing.T) {
	t.Run("When action exists it should return it", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions/get_pods" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Action{Name: "get_pods", Scope: "kube-api", Type: "read"})
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		action, err := c.GetAction(context.Background(), "get_pods")
		if err != nil {
			t.Fatalf("GetAction() error = %v", err)
		}
		if action.Name != "get_pods" {
			t.Errorf("expected name 'get_pods', got %q", action.Name)
		}
	})
}

func TestClientListActions(t *testing.T) {
	t.Run("When actions exist it should return list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ActionList{Items: []Action{
				{Name: "get_pods"}, {Name: "rollout_restart"},
			}})
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		list, err := c.ListActions(context.Background())
		if err != nil {
			t.Fatalf("ListActions() error = %v", err)
		}
		if len(list.Items) != 2 {
			t.Errorf("expected 2 actions, got %d", len(list.Items))
		}
	})
}

func TestClientListAudit(t *testing.T) {
	t.Run("When query params provided it should pass them", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions/audit" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.Query().Get("since") != "24h" {
				t.Errorf("expected since=24h, got %q", r.URL.Query().Get("since"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(AuditList{Items: []AuditEntry{{Method: "POST", StatusCode: 200}}})
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		q := make(map[string][]string)
		q["since"] = []string{"24h"}
		list, err := c.ListAudit(context.Background(), q)
		if err != nil {
			t.Fatalf("ListAudit() error = %v", err)
		}
		if len(list.Items) != 1 {
			t.Errorf("expected 1 entry, got %d", len(list.Items))
		}
	})
}

func TestClientServerVersion(t *testing.T) {
	t.Run("When server responds it should parse version info", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/version" {
				t.Errorf("expected path '/version', got %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ServerVersionInfo{
				Version: "0.5.0", GitCommit: "abc123", GoVersion: "go1.22", Platform: "linux/amd64", Target: "eph-test-rc",
			})
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		info, err := c.ServerVersion(context.Background())
		if err != nil {
			t.Fatalf("ServerVersion() error = %v", err)
		}
		if info.Version != "0.5.0" {
			t.Errorf("expected version '0.5.0', got %q", info.Version)
		}
		if info.Target != "eph-test-rc" {
			t.Errorf("expected target 'eph-test-rc', got %q", info.Target)
		}
	})
}

func TestClientRawGet(t *testing.T) {
	t.Run("When server responds it should return raw response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions/runs/exec-1/output" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":"raw content"}`))
		}))
		defer server.Close()

		c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
		resp, err := c.RawGet(context.Background(), "/trusted-actions/runs/exec-1/output")
		if err != nil {
			t.Fatalf("RawGet() error = %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestClientDoRoot_WhenLambdaRuntimeErrorOn200_ItShouldReturnLambdaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/targets" {
			t.Fatalf("path = %q, want /targets", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errorMessage":"RequestId: abc","errorType":"Runtime.InvalidEntrypoint"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, region: "us-east-1", sigService: "lambda", credentials: staticCredentials{}, httpClient: http.DefaultClient}
	_, err := c.ListTargets(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	lambdaErr, ok := err.(*LambdaRuntimeError)
	if !ok {
		t.Fatalf("expected *LambdaRuntimeError, got %T: %v", err, err)
	}
	if lambdaErr.ErrorType != "Runtime.InvalidEntrypoint" {
		t.Errorf("expected Runtime.InvalidEntrypoint, got %q", lambdaErr.ErrorType)
	}
	if lambdaErr.Surface != APISurfaceAccess {
		t.Errorf("Surface = %q, want %q", lambdaErr.Surface, APISurfaceAccess)
	}
	if !strings.Contains(lambdaErr.Error(), "ZOA Access API") {
		t.Errorf("Error() = %q, want ZOA Access API prefix", lambdaErr.Error())
	}
}

func TestClientDo_WhenLambdaRuntimeError_ItShouldReturnLambdaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errorMessage":"exit status 1","errorType":"Runtime.ExitError"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
	_, err := c.ListActions(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	lambdaErr, ok := err.(*LambdaRuntimeError)
	if !ok {
		t.Fatalf("expected *LambdaRuntimeError, got %T: %v", err, err)
	}
	if lambdaErr.ErrorType != "Runtime.ExitError" {
		t.Errorf("expected Runtime.ExitError, got %q", lambdaErr.ErrorType)
	}
	if lambdaErr.Surface != APISurfaceAPIPlane {
		t.Errorf("Surface = %q, want %q", lambdaErr.Surface, APISurfaceAPIPlane)
	}
}

func TestClientDo_WhenGenericHTTPError_ItShouldReturnFormattedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Bad Gateway"))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, region: "us-east-1", credentials: staticCredentials{}, httpClient: http.DefaultClient}
	_, err := c.ListActions(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*APIError); ok {
		t.Error("should NOT be APIError for plain text response")
	}
}

func TestClientWithTimeout(t *testing.T) {
	c := &Client{baseURL: "https://test.lambda-url.us-east-1.on.aws", timeout: 30 * time.Second}
	c2 := c.WithTimeout(5 * time.Second)
	if c2.timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", c2.timeout)
	}
	if c.timeout != 30*time.Second {
		t.Error("original client should not be modified")
	}
}

func TestClientNew(t *testing.T) {
	t.Run("When valid Lambda URL it should succeed", func(t *testing.T) {
		c, err := New("https://abc.lambda-url.us-east-1.on.aws", staticCredentials{}, Options{
			AccountID: "123456",
			Operator:  "arn:aws:iam::123456:user/test",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.region != "us-east-1" {
			t.Errorf("expected region us-east-1, got %q", c.region)
		}
		if c.accountID != "123456" {
			t.Errorf("expected accountID '123456', got %q", c.accountID)
		}
	})

	t.Run("When custom CNAME without region option it should fail", func(t *testing.T) {
		_, err := New("https://zoa.internal.example.com", staticCredentials{}, Options{})
		if err == nil {
			t.Fatal("expected error for custom CNAME without region")
		}
	})

	t.Run("When custom CNAME with region option it should succeed", func(t *testing.T) {
		c, err := New("https://zoa.internal.example.com", staticCredentials{}, Options{Region: "eu-west-1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.region != "eu-west-1" {
			t.Errorf("expected region eu-west-1, got %q", c.region)
		}
	})
}

func TestClientDo_WhenHeadersSet_ItShouldSendAccountAndOperator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Account-ID") != "999888" {
			t.Errorf("expected X-Account-ID=999888, got %q", r.Header.Get("X-Account-ID"))
		}
		if r.Header.Get("X-Operator") != "arn:aws:iam::999888:user/sre" {
			t.Errorf("expected X-Operator header, got %q", r.Header.Get("X-Operator"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ActionList{Items: []Action{}})
	}))
	defer server.Close()

	c := &Client{
		baseURL:     server.URL,
		region:      "us-east-1",
		credentials: staticCredentials{},
		httpClient:  http.DefaultClient,
		accountID:   "999888",
		operator:    "arn:aws:iam::999888:user/sre",
	}
	_, err := c.ListActions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- sha256Hash tests ---

func TestSha256Hash_WhenValidInput_ItShouldReturnCorrectHash(t *testing.T) {
	input := strings.NewReader(`{"action":"test"}`)
	hash := sha256Hash(input)
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex string, got len=%d: %q", len(hash), hash)
	}

	// After hashing, reader should be reset to start
	buf := make([]byte, 20)
	n, _ := input.Read(buf)
	if string(buf[:n]) != `{"action":"test"}` {
		t.Errorf("reader was not reset after hashing")
	}
}

func TestSha256Hash_WhenEmptyInput_ItShouldReturnEmptySha(t *testing.T) {
	input := strings.NewReader("")
	hash := sha256Hash(input)
	// SHA-256 of empty string
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if hash != expected {
		t.Errorf("sha256('') = %q, want %q", hash, expected)
	}
}

func TestSha256Hash_WhenCalledTwice_ItShouldProduceSameResult(t *testing.T) {
	input := strings.NewReader("deterministic content")
	h1 := sha256Hash(input)
	h2 := sha256Hash(input)
	if h1 != h2 {
		t.Errorf("sha256Hash not deterministic: %q != %q", h1, h2)
	}
}
