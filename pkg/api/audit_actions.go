package api

// Audit action strings for API Lambda meta-operations (not Trusted Action registry names).
// Access plane uses session_* via recordAccessAudit. TA dispatch uses the TA name.
//
// Taxonomy:
//   - session_*     — Access Lambda (lifecycle + privileged session reads)
//   - <ta_name>     — POST .../trusted-actions/{ta}/run
//   - runs_* / run_* — execution plane reads (list/get/artifacts)
//   - audit_*       — reading the audit index itself
const (
	AuditActionQuery     = "audit_query"
	AuditActionRunsList  = "runs_list"
	AuditActionRunGet    = "run_get"
	AuditActionRunOutput = "run_output"
	AuditActionRunLogs   = "run_logs"

	AuditActionBreakglassScopes  = "breakglass_scopes"
	AuditActionBreakglassRequest = "breakglass_request"
	AuditActionBreakglassConnect = "breakglass_connect"
	AuditActionBreakglassApprove = "breakglass_approve"
	AuditActionBreakglassReject  = "breakglass_reject"
	AuditActionBreakglassHistory = "breakglass_history"
)
