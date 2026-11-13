package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/output"
)

// renderBreakglassRequest is for create/--wait ack flows (stderr ✓ line + hints). Not used for passive get.

func breakglassProgressWriter() io.Writer {
	return os.Stderr
}

func breakglassTargetCluster(cluster string) string {
	if cluster != "" {
		return cluster
	}
	if t := os.Getenv("ZOA_TARGET"); t != "" {
		return t
	}
	return "unknown"
}

// printBreakglassRequestDispatched mirrors printSessionDispatched: audit id + target on stderr.
func printBreakglassRequestDispatched(w io.Writer, req *client.BreakglassRequest) {
	if req == nil || req.RequestID == "" {
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = "unknown"
	}
	fmt.Fprintf(w, "✓ %s [%s] scope=%s\n", req.RequestID, breakglassTargetCluster(req.TargetCluster), scope)
}

// printBreakglassActivateDispatched marks successful activate (same id line + · activated).
func printBreakglassActivateDispatched(w io.Writer, requestID, targetCluster, scope string) {
	if requestID == "" {
		return
	}
	if scope == "" {
		scope = "unknown"
	}
	fmt.Fprintf(w, "✓ %s [%s] scope=%s · activated\n", requestID, breakglassTargetCluster(targetCluster), scope)
}

func breakglassApprovalDeployment() string {
	if d := os.Getenv("ZOA_DEPLOYMENT"); d != "" {
		return d
	}
	return "<deployment>"
}

// renderBreakglassRequest prints progress + optional human detail after create/get/--wait (never activates).
func renderBreakglassRequest(global *GlobalOptions, req *client.BreakglassRequest, timedOut bool) error {
	if req == nil {
		return fmt.Errorf("break-glass request is nil")
	}
	deployment := breakglassApprovalDeployment()
	next := breakglassRequestNextStep(req, deployment, timedOut)
	progress := breakglassProgressWriter()
	printBreakglassRequestDispatched(progress, req)

	if global.OutputFormat == output.FormatJSON {
		payload := map[string]interface{}{
			"id":                      req.RequestID,
			"scope":                   req.Scope,
			"target_cluster":          req.TargetCluster,
			"status":                  req.Status,
			"reason":                  req.Reason,
			"operator":                req.Operator,
			"grant_expires_at":        req.GrantExpiresAt,
			"request_expires_at":      req.RequestExpiresAt,
			"approval_wait_timed_out": timedOut && req.Status == "pending_approval",
			"next_step":               next,
		}
		return output.JSON(os.Stdout, payload)
	}

	printBreakglassRequestDetail(progress, req, deployment, timedOut, next)
	return nil
}

func breakglassRequestNextStep(req *client.BreakglassRequest, deployment string, timedOut bool) string {
	switch req.Status {
	case "approved":
		return fmt.Sprintf("zoa breakglass activate %s", req.RequestID)
	case "pending_approval":
		if timedOut {
			return fmt.Sprintf("zoa breakglass get %s --wait", req.RequestID)
		}
		return fmt.Sprintf("Peer: zoa approve %s/%s", deployment, req.RequestID)
	case "rejected", "expired":
		return ""
	default:
		return fmt.Sprintf("zoa breakglass get %s", req.RequestID)
	}
}

func printBreakglassRequestDetail(w io.Writer, req *client.BreakglassRequest, deployment string, timedOut bool, next string) {
	fmt.Fprintf(w, "  status: %s\n", req.Status)
	if req.GrantExpiresAt != "" {
		fmt.Fprintf(w, "  grant expires (UTC): %s\n", req.GrantExpiresAt)
	}
	if req.RequestExpiresAt != "" && req.Status == "pending_approval" {
		fmt.Fprintf(w, "  approval deadline (UTC): %s\n", req.RequestExpiresAt)
	}

	switch req.Status {
	case "approved":
		fmt.Fprintf(w, "  run: %s\n", next)
	case "pending_approval":
		if timedOut {
			fmt.Fprintln(w, "  still pending approval (poll timed out)")
		}
		fmt.Fprintf(w, "  peer approve: zoa approve %s/%s\n", deployment, req.RequestID)
		fmt.Fprintf(w, "  track:        zoa breakglass get %s --wait\n", req.RequestID)
	case "rejected":
		fmt.Fprintln(w, "  rejected — do not activate")
	case "expired":
		fmt.Fprintln(w, "  expired — create a new request if still needed")
	}
}

func printBreakglassRequestHints(w io.Writer, req *client.BreakglassRequest) {
	if req == nil {
		return
	}
	deployment := breakglassApprovalDeployment()
	switch req.Status {
	case "approved":
		fmt.Fprintf(w, "  run: zoa breakglass activate %s\n", req.RequestID)
	case "pending_approval":
		fmt.Fprintf(w, "  peer approve: zoa approve %s/%s\n", deployment, req.RequestID)
		fmt.Fprintf(w, "  track:        zoa breakglass get %s --wait\n", req.RequestID)
	}
}

// renderBreakglassGetHuman mirrors renderExecution (zoa get): metadata on stdout, no dispatch ack line.
func renderBreakglassGetHuman(w io.Writer, req *client.BreakglassRequest, timedOut bool) {
	if req == nil {
		return
	}
	deployment := breakglassApprovalDeployment()
	next := breakglassRequestNextStep(req, deployment, timedOut)

	fmt.Fprintf(w, "ID:        %s\n", req.RequestID)
	fmt.Fprintf(w, "SCOPE:     %s\n", req.Scope)
	fmt.Fprintf(w, "TARGET:    %s\n", breakglassTargetCluster(req.TargetCluster))
	fmt.Fprintf(w, "STATUS:    %s\n", req.Status)
	fmt.Fprintf(w, "REASON:    %s\n", output.Dash(req.Reason))
	fmt.Fprintf(w, "OPERATOR:  %s\n", output.Dash(req.Operator))
	if req.SessionID != "" {
		fmt.Fprintf(w, "SESSION:   %s\n", req.SessionID)
	}
	if req.SignerARN != "" {
		fmt.Fprintf(w, "SIGNER:    %s\n", req.SignerARN)
	}
	if req.TargetType != "" {
		fmt.Fprintf(w, "TYPE:      %s\n", req.TargetType)
	}
	if req.Approval != "" {
		fmt.Fprintf(w, "APPROVAL:  %s\n", req.Approval)
	}
	fmt.Fprintf(w, "CREATED:   %s\n", formatBreakglassTime(req.CreatedAt))
	if req.GrantExpiresAt != "" {
		fmt.Fprintf(w, "GRANT_EXPIRES:     %s\n", formatBreakglassTime(req.GrantExpiresAt))
	}
	if req.RequestExpiresAt != "" {
		fmt.Fprintf(w, "APPROVAL_DEADLINE: %s\n", formatBreakglassTime(req.RequestExpiresAt))
	}
	if req.ApprovedBy != "" {
		fmt.Fprintf(w, "APPROVED_BY: %s\n", req.ApprovedBy)
		fmt.Fprintf(w, "APPROVED_AT: %s\n", formatBreakglassTime(req.ApprovedAt))
	}
	if req.RejectedBy != "" {
		fmt.Fprintf(w, "REJECTED_BY: %s\n", req.RejectedBy)
		fmt.Fprintf(w, "REJECTED_AT: %s\n", formatBreakglassTime(req.RejectedAt))
	}
	if req.DecisionNote != "" {
		fmt.Fprintf(w, "NOTE:      %s\n", req.DecisionNote)
	}
	fmt.Fprintf(w, "REVISION:  %s\n", output.Dash(req.Revision))
	if timedOut && req.Status == "pending_approval" {
		fmt.Fprintln(w, "NOTE:     poll timed out; still pending_approval")
	}
	if next != "" {
		fmt.Fprintf(w, "NEXT:     %s\n", next)
	}
}

func breakglassWaitApprovalError(req *client.BreakglassRequest, timedOut bool) error {
	switch req.Status {
	case "rejected":
		return fmt.Errorf("break-glass request %s was rejected", req.RequestID)
	case "expired":
		return fmt.Errorf("break-glass request %s expired before approval", req.RequestID)
	case "pending_approval":
		if timedOut {
			return fmt.Errorf("break-glass request %s still pending_approval after wait timeout", req.RequestID)
		}
		return fmt.Errorf("break-glass request %s not approved (status %s)", req.RequestID, req.Status)
	case "approved":
		return nil
	default:
		return fmt.Errorf("break-glass request %s unexpected status %q after wait", req.RequestID, req.Status)
	}
}
