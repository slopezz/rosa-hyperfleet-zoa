package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Execution struct {
	ID              string            `json:"id"`
	Action          string            `json:"action"`
	RequestedAction string            `json:"requested_action,omitempty"`
	TargetCluster   string            `json:"target_cluster"`
	Status          string            `json:"status"`
	ExecutionMode   string            `json:"execution_mode,omitempty"`
	Scope           string            `json:"scope"`
	Type            string            `json:"type"`
	DryRun          bool              `json:"dry_run"`
	Force           bool              `json:"force"`
	Jira            string            `json:"jira,omitempty"`
	Operator        string            `json:"operator,omitempty"`
	Revision        string            `json:"revision,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	CreatedAt       *time.Time        `json:"created_at,omitempty"`
	DispatchedAt    *time.Time        `json:"dispatched_at,omitempty"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
	DurationMs      *int64            `json:"duration_ms,omitempty"`
	OutputBytes     *int64            `json:"output_bytes,omitempty"`
	LogBytes        *int64            `json:"log_bytes,omitempty"`
	OutputFormat    string            `json:"output_format,omitempty"`
	Output          FlexString        `json:"output,omitempty"`
	Logs            string            `json:"logs,omitempty"`
}

// FlexString handles API fields that may be a string, array, or object.
// When unmarshaled, it stores the raw string content for human rendering.
// When marshaled back to JSON, it re-parses to emit proper nested JSON.
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(string(data))
	return nil
}

func (f FlexString) MarshalJSON() ([]byte, error) {
	s := string(f)
	if s == "" {
		return []byte("null"), nil
	}
	trimmed := strings.TrimSpace(s)
	if (strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{")) && json.Valid([]byte(trimmed)) {
		return []byte(trimmed), nil
	}
	return json.Marshal(s)
}

func (f FlexString) String() string {
	return string(f)
}

type ExecutionList struct {
	Items     []Execution `json:"items"`
	Count     int         `json:"count,omitempty"`
	NextToken *string     `json:"next_token,omitempty"`
}

type DispatchRequest struct {
	Jira           string            `json:"jira"`
	Params         map[string]string `json:"params,omitempty"`
	Force          bool              `json:"force"`
	DryRun         bool              `json:"dry_run"`
	ExecutionMode  string            `json:"execution_mode,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

type DispatchResponse struct {
	ID              string     `json:"id"`
	Action          string     `json:"action"`
	RequestedAction string     `json:"requested_action,omitempty"`
	TargetCluster   string     `json:"target_cluster"`
	Operator        string     `json:"operator"`
	Status          string     `json:"status"`
	ExecutionMode   string     `json:"execution_mode,omitempty"`
	Scope           string     `json:"scope"`
	Type            string     `json:"type"`
	DryRun          bool       `json:"dry_run"`
	Force           bool       `json:"force"`
	Output          FlexString `json:"output,omitempty"`
	Logs            string     `json:"logs,omitempty"`
	DurationMs      *int64     `json:"duration_ms,omitempty"`
}

type ActionParam struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

type ActionAuthorization struct {
	Approval string `json:"approval,omitempty"`
}

type Action struct {
	Name                 string              `json:"name"`
	Scope                string              `json:"scope"`
	Type                 string              `json:"type"`
	ExecutionMode        string              `json:"execution_mode,omitempty"`
	Description          string              `json:"description"`
	Params               []ActionParam       `json:"parameters,omitempty"`
	Authorization        ActionAuthorization `json:"authorization,omitempty"`
	DryRunAction         string              `json:"dry_run_action,omitempty"`
	WriteCooldownSeconds int                 `json:"write_cooldown_seconds,omitempty"`
	TimeoutSeconds       int                 `json:"timeout_seconds,omitempty"`
}

type ActionList struct {
	Items []Action `json:"items"`
}

type AuditEntry struct {
	Timestamp     string `json:"timestamp"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	StatusCode    int    `json:"status_code"`
	Operator      string `json:"operator"`
	Action        string `json:"action,omitempty"`
	TargetCluster string `json:"target_cluster,omitempty"`
	SourceIP      string `json:"source_ip,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	UserAgent     string `json:"user_agent,omitempty"`
	Jira          string `json:"jira,omitempty"`
	Force         bool   `json:"force,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
	ApprovalState string `json:"approval_state,omitempty"`
	ExecutionID   string `json:"execution_id,omitempty"`
}

func (a AuditEntry) ShortPath() string {
	if strings.HasPrefix(a.Path, "/api/v0/trusted-actions/") {
		return a.Path[len("/api/v0/trusted-actions/"):]
	}
	return a.Path
}

type AuditList struct {
	Items []AuditEntry `json:"items"`
}

type ServerVersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	Target    string `json:"target"`
}

// APISurface identifies which ZOA HTTP endpoint returned an error (for CLI logs and bug reports).
type APISurface string

const (
	// APISurfaceZOA is the unified label for all ZOA v0 HTTP APIs (TA and Access planes).
	APISurfaceZOA APISurface = "ZOA API"
	// APISurfaceAPI is an alias for APISurfaceZOA (Trusted Action Lambda).
	APISurfaceAPI = APISurfaceZOA
	// APISurfaceAccess is an alias for APISurfaceZOA (Access Lambda).
	APISurfaceAccess = APISurfaceZOA
)

type APIError struct {
	Code       string     `json:"code"`
	Reason     string     `json:"reason"`
	Message    string     `json:"message,omitempty"`
	Surface    APISurface `json:"-"`
	HTTPStatus int        `json:"-"`
}

func (e *APIError) Error() string {
	var msg string
	if e.Reason != "" {
		msg = e.Reason
	} else if e.Message != "" {
		msg = e.Message
	} else {
		msg = e.Code
	}
	return formatClientError(e.Surface, e.HTTPStatus, msg)
}

// LambdaRuntimeError is returned by AWS Lambda Function URLs when the Lambda
// function crashes, times out, or returns an unhandled error. The format differs
// from ZOA's APIError.
type LambdaRuntimeError struct {
	ErrorMessage string     `json:"errorMessage"`
	ErrorType    string     `json:"errorType"`
	Surface      APISurface `json:"-"`
	HTTPStatus   int        `json:"-"`
}

func formatClientError(surface APISurface, httpStatus int, detail string) string {
	if surface != "" && httpStatus > 0 {
		return fmt.Sprintf("%s (HTTP %d): %s", surface, httpStatus, detail)
	}
	if surface != "" {
		return string(surface) + ": " + detail
	}
	if httpStatus > 0 {
		return fmt.Sprintf("HTTP %d: %s", httpStatus, detail)
	}
	return detail
}

func (e *LambdaRuntimeError) awsFacts() string {
	if e.ErrorMessage != "" {
		return fmt.Sprintf("aws %s: %s", e.ErrorType, e.ErrorMessage)
	}
	return fmt.Sprintf("aws %s", e.ErrorType)
}

func (e *LambdaRuntimeError) Error() string {
	switch e.ErrorType {
	case "Runtime.ExitError":
		detail := fmt.Sprintf("%s — Lambda failed to start; check CloudWatch", e.awsFacts())
		return formatClientError(e.Surface, e.HTTPStatus, detail)
	case "Runtime.InvalidEntrypoint":
		detail := fmt.Sprintf("%s — Lambda did not start; check function CPU arch vs image and CloudWatch", e.awsFacts())
		return formatClientError(e.Surface, e.HTTPStatus, detail)
	case "Runtime.DeadlineExceeded":
		detail := fmt.Sprintf("%s — Lambda execution deadline exceeded", e.awsFacts())
		return formatClientError(e.Surface, e.HTTPStatus, detail)
	default:
		return formatClientError(e.Surface, e.HTTPStatus, e.awsFacts())
	}
}

func (e *LambdaRuntimeError) IsUnavailable() bool {
	return e.ErrorType == "Runtime.ExitError" || e.ErrorType == "Runtime.InvalidEntrypoint"
}

// --- Access / Boundary types ---

type Target struct {
	TargetID       string `json:"target_id"`
	DeploymentName string `json:"deployment_name"`
	VpcId          string `json:"vpc_id,omitempty"`
	TargetType     string `json:"target_type"`
	Region         string `json:"region"`
	Status         string `json:"status,omitempty"`
	FunctionUrl    string `json:"function_url,omitempty"`
}

type TargetList struct {
	Items []Target `json:"items"`
	Count int      `json:"count"`
}

type Session struct {
	SessionID   string `json:"session_id"`
	Operator    string `json:"operator"`
	Target      string `json:"target_cluster"`
	Status      string `json:"status"`
	Region      string `json:"region,omitempty"`
	EcsCluster  string `json:"ecs_cluster,omitempty"`
	TaskArn     string `json:"task_arn,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Deadline    string `json:"deadline,omitempty"`
	CompletedAt string `json:"terminated_at,omitempty"`
}

type SessionList struct {
	Items []Session `json:"items"`
	Count int       `json:"count"`
}

type SessionStartRequest struct {
	DeploymentName string `json:"deployment_name"`
	Target         string `json:"target"`
}

type SessionStartResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	TaskArn   string `json:"task_arn,omitempty"`
	Region    string `json:"region,omitempty"`
}

type SessionJoinResponse struct {
	SessionID     string `json:"session_id"`
	TaskArn       string `json:"task_arn"`
	ClusterArn    string `json:"cluster"`
	ContainerName string `json:"container_name"`
	Region        string `json:"region"`
}
