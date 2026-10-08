package store

import "time"

type Status string

const (
	StatusPendingApproval Status = "pending_approval"
	StatusApproved        Status = "approved"
	StatusRejected        Status = "rejected"
	StatusDispatched      Status = "dispatched"
	StatusSucceeded       Status = "succeeded"
	StatusFailed          Status = "failed"
	StatusTimedOut        Status = "timed_out"
)

func (s Status) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusTimedOut || s == StatusRejected
}

type Execution struct {
	ID              string            `json:"id" dynamodbav:"executionId"`
	Action          string            `json:"action" dynamodbav:"action"`
	RequestedAction string            `json:"requested_action,omitempty" dynamodbav:"requestedAction,omitempty"`
	AccountID       string            `json:"account_id" dynamodbav:"accountId"`
	TargetCluster   string            `json:"target_cluster" dynamodbav:"targetCluster"`
	Operator        string            `json:"operator" dynamodbav:"operator"`
	SignerARN       string            `json:"signer_arn,omitempty" dynamodbav:"signerARN,omitempty"`
	SessionID       string            `json:"session_id,omitempty" dynamodbav:"sessionId,omitempty"`
	Status          Status            `json:"status" dynamodbav:"status"`
	ExecutionMode   string            `json:"execution_mode" dynamodbav:"executionMode"`
	Scope           string            `json:"scope" dynamodbav:"scope"`
	Type            string            `json:"type" dynamodbav:"type"`
	DryRun          bool              `json:"dry_run" dynamodbav:"dryRun"`
	Force           bool              `json:"force" dynamodbav:"force"`
	Params          map[string]string `json:"params,omitempty" dynamodbav:"params,omitempty"`
	Reason          string            `json:"reason,omitempty" dynamodbav:"reason,omitempty"`

	Revision string `json:"revision,omitempty" dynamodbav:"revision,omitempty"`

	TimeoutSeconds int    `json:"timeout_seconds" dynamodbav:"timeoutSeconds"`
	CreatedAt      string `json:"created_at" dynamodbav:"createdAt"`
	DispatchedAt   string `json:"dispatched_at,omitempty" dynamodbav:"dispatchedAt,omitempty"`
	CompletedAt    string `json:"completed_at,omitempty" dynamodbav:"completedAt,omitempty"`
	DurationMs     int64  `json:"duration_ms,omitempty" dynamodbav:"durationMs,omitempty"`

	Cleaned   bool   `json:"cleaned" dynamodbav:"cleaned"`
	CleanedAt string `json:"cleaned_at,omitempty" dynamodbav:"cleanedAt,omitempty"`

	OutputBytes  int64  `json:"output_bytes,omitempty" dynamodbav:"outputBytes,omitempty"`
	LogBytes     int64  `json:"log_bytes,omitempty" dynamodbav:"logBytes,omitempty"`
	OutputFormat string `json:"output_format,omitempty" dynamodbav:"outputFormat,omitempty"`

	ApprovedBy string `json:"approved_by,omitempty" dynamodbav:"approvedBy,omitempty"`

	TargetStatusKey string `json:"-" dynamodbav:"targetStatusKey,omitempty"`
	DateBucket      string `json:"-" dynamodbav:"dateBucket,omitempty"`

	TTL int64 `json:"-" dynamodbav:"ttl,omitempty"`
}

func (e *Execution) CreatedAtTime() (time.Time, error) {
	return time.Parse(time.RFC3339Nano, e.CreatedAt)
}

func (e *Execution) DispatchedAtTime() (time.Time, error) {
	return time.Parse(time.RFC3339Nano, e.DispatchedAt)
}

type AuditEntry struct {
	AccountID     string `json:"account_id" dynamodbav:"accountId"`
	Timestamp     string `json:"timestamp" dynamodbav:"timestamp"`
	Method        string `json:"method" dynamodbav:"method"`
	Path          string `json:"path" dynamodbav:"path"`
	StatusCode    int    `json:"status_code" dynamodbav:"statusCode"`
	Operator      string `json:"operator" dynamodbav:"operator"`
	SignerARN     string `json:"signer_arn,omitempty" dynamodbav:"signerARN,omitempty"`
	SessionID     string `json:"session_id,omitempty" dynamodbav:"sessionId,omitempty"`
	Action        string `json:"action,omitempty" dynamodbav:"action,omitempty"`
	TargetCluster string `json:"target_cluster,omitempty" dynamodbav:"targetCluster,omitempty"`
	SourceIP      string `json:"source_ip,omitempty" dynamodbav:"sourceIp,omitempty"`
	RequestID     string `json:"request_id,omitempty" dynamodbav:"requestId,omitempty"`
	UserAgent     string `json:"user_agent,omitempty" dynamodbav:"userAgent,omitempty"`
	Reason        string `json:"reason,omitempty" dynamodbav:"reason,omitempty"`
	Force         bool   `json:"force,omitempty" dynamodbav:"force,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty" dynamodbav:"dryRun,omitempty"`
	ExecutionID   string `json:"execution_id,omitempty" dynamodbav:"executionId,omitempty"`
	DateBucket    string `json:"-" dynamodbav:"dateBucket,omitempty"`
	TTL           int64  `json:"-" dynamodbav:"ttl,omitempty"`
}

type ListFilter struct {
	Target        *string
	Status        *Status
	ExecutionMode *string
	Action        *string
	Type          *string
	Scope         *string
	Operator      *string
	DryRun        *bool
	Force         *bool
	Since         *time.Time
	Before        *time.Time
	Limit         int
}

type AuditFilter struct {
	Target   *string
	Since    *time.Time
	Before   *time.Time
	Action   *string
	Method   *string
	Operator *string
	Force    *bool
	DryRun   *bool
	Limit    int
}
