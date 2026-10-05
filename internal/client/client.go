package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

type Client struct {
	baseURL     string
	region      string
	sigService  string
	credentials aws.CredentialsProvider
	httpClient  *http.Client
	signer      *v4.Signer
	accountID   string
	operator    string
	timeout     time.Duration
}

// Options configures the ZOA client.
type Options struct {
	AccountID string // AWS account ID (derived from STS if empty)
	Operator  string // SRE identity (IAM ARN from STS GetCallerIdentity)
	Region    string // AWS region override (auto-detected from URL if empty)
}

func New(apiURL string, creds aws.CredentialsProvider, opts Options) (*Client, error) {
	region, service, err := parseEndpoint(apiURL)
	if err != nil && opts.Region == "" {
		return nil, err
	}
	if opts.Region != "" {
		region = opts.Region
	}

	return &Client{
		baseURL:     strings.TrimRight(apiURL, "/"),
		region:      region,
		sigService:  service,
		credentials: creds,
		httpClient:  &http.Client{},
		signer:      v4.NewSigner(),
		accountID:   opts.AccountID,
		operator:    opts.Operator,
		timeout:     30 * time.Second,
	}, nil
}

// WithTimeout returns a shallow copy of the Client with the given request timeout.
func (c *Client) WithTimeout(d time.Duration) *Client {
	cc := *c
	cc.timeout = d
	return &cc
}

// parseEndpoint extracts the AWS region and SigV4 service name from a URL.
// Supported formats:
//   - API Gateway:       <id>.execute-api.<region>.amazonaws.com  → service "execute-api"
//   - Lambda Function URL: <id>.lambda-url.<region>.on.aws        → service "lambda"
//   - Custom CNAME:      anything else                            → service "lambda", region must be provided via Options.Region
func parseEndpoint(apiURL string) (region, service string, err error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid API URL: %w", err)
	}
	parts := strings.Split(u.Host, ".")
	for i, p := range parts {
		if p == "execute-api" && i+1 < len(parts) {
			return parts[i+1], "execute-api", nil
		}
		if p == "lambda-url" && i+1 < len(parts) {
			return parts[i+1], "lambda", nil
		}
	}
	return "", "lambda", fmt.Errorf("cannot auto-detect region from URL %q; set AWS_REGION or use --region", apiURL)
}

func (c *Client) Dispatch(ctx context.Context, action string, req *DispatchRequest) (*DispatchResponse, error) {
	// Sync TAs can run up to the Lambda deadline (~15min); use an extended client timeout.
	longClient := c.WithTimeout(16 * time.Minute)
	var resp DispatchResponse
	if err := longClient.doV0(ctx, http.MethodPost, "/trusted-actions/"+url.PathEscape(action)+"/run", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetExecution(ctx context.Context, id string, include string) (*Execution, error) {
	path := "/trusted-actions/runs/" + url.PathEscape(id)
	if include != "" {
		q := url.Values{}
		q.Set("include", include)
		path += "?" + q.Encode()
	}
	var resp Execution
	if err := c.doV0(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListExecutions(ctx context.Context, query url.Values) (*ExecutionList, error) {
	path := "/trusted-actions/runs"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var resp ExecutionList
	if err := c.doV0(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetAction(ctx context.Context, name string) (*Action, error) {
	var resp Action
	if err := c.doV0(ctx, http.MethodGet, "/trusted-actions/"+url.PathEscape(name), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListActions(ctx context.Context) (*ActionList, error) {
	var resp ActionList
	if err := c.doV0(ctx, http.MethodGet, "/trusted-actions", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListAudit(ctx context.Context, query url.Values) (*AuditList, error) {
	path := "/trusted-actions/audit"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var resp AuditList
	if err := c.doV0(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ServerVersion fetches the server's /version endpoint.
func (c *Client) ServerVersion(ctx context.Context) (*ServerVersionInfo, error) {
	var info ServerVersionInfo
	if err := c.do(ctx, http.MethodGet, "/version", nil, &info, rootRequest()); err != nil {
		return nil, err
	}
	return &info, nil
}

// RawGet performs a signed GET request and returns the raw HTTP response.
// Caller is responsible for closing resp.Body. Uses a 10-minute timeout
// as a safety net for large artifact downloads.
func (c *Client) RawGet(ctx context.Context, path string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	_ = cancel // caller owns resp.Body; context will be cancelled when resp.Body is closed or deferred

	fullURL := c.baseURL + "/api/v0" + path

	bodyReader := bytes.NewReader(nil)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, bodyReader)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if err := c.signRequest(ctx, req, bodyReader); err != nil {
		cancel()
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return resp, nil
}

// decodeResponse parses Function URL / API responses. Lambda may return runtime
// error JSON (errorType/errorMessage) with HTTP 200 on streaming URLs — never
// treat that as a successful empty API payload.
func decodeResponse(surface APISurface, statusCode int, respBody []byte, result any) error {
	if len(respBody) > 0 {
		var lambdaErr LambdaRuntimeError
		if json.Unmarshal(respBody, &lambdaErr) == nil && lambdaErr.ErrorType != "" {
			lambdaErr.Surface = surface
			lambdaErr.HTTPStatus = statusCode
			return &lambdaErr
		}
	}
	if statusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Code != "" {
			apiErr.Surface = surface
			apiErr.HTTPStatus = statusCode
			return &apiErr
		}
		body := string(respBody)
		if len(body) > 512 {
			body = body[:512] + "...(truncated)"
		}
		return fmt.Errorf("%s", formatClientError(surface, statusCode, body))
	}
	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// --- Access / Boundary methods (same /api/v0 prefix as Trusted Actions) ---

func (c *Client) ListTargets(ctx context.Context) (*TargetList, error) {
	var resp TargetList
	if err := c.doV0(ctx, http.MethodGet, "/targets", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListTargetsByDeployment(ctx context.Context, deployment string) (*TargetList, error) {
	var resp TargetList
	if err := c.doV0(ctx, http.MethodGet, "/targets/"+url.PathEscape(deployment), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SessionStart(ctx context.Context, req *SessionStartRequest) (*SessionStartResponse, error) {
	var resp SessionStartResponse
	if err := c.doV0(ctx, http.MethodPost, "/sessions/start", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SessionStop(ctx context.Context, sessionID string) error {
	return c.doV0(ctx, http.MethodPost, "/sessions/stop/"+url.PathEscape(sessionID), nil, nil)
}

func (c *Client) SessionJoin(ctx context.Context, sessionID string) (*SessionJoinResponse, error) {
	var resp SessionJoinResponse
	if err := c.doV0(ctx, http.MethodPost, "/sessions/join/"+url.PathEscape(sessionID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SessionExecAttached(ctx context.Context, sessionID, execSessionID string) error {
	body := map[string]string{"exec_session_id": execSessionID}
	return c.doV0(ctx, http.MethodPost, "/sessions/exec-attached/"+url.PathEscape(sessionID), body, nil)
}

func (c *Client) ListSessions(ctx context.Context, query url.Values) (*SessionList, error) {
	path := "/sessions"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var resp SessionList
	if err := c.doV0(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func sha256Hash(reader io.ReadSeeker) string {
	h := sha256.New()
	_, _ = io.Copy(h, reader)
	_, _ = reader.Seek(0, io.SeekStart)
	return hex.EncodeToString(h.Sum(nil))
}
