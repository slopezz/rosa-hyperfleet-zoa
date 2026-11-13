package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func breakglassLifecycleStatus(req *client.BreakglassRequest) string {
	if req == nil || req.Status == "" {
		return "-"
	}
	return req.Status
}

func breakglassWorkflowApproval(req *client.BreakglassRequest) string {
	if req == nil || req.Approval == "" {
		return "-"
	}
	return req.Approval
}

func breakglassDecidedAt(req *client.BreakglassRequest) string {
	if req == nil {
		return ""
	}
	return req.DecidedAt
}

func breakglassDecidedBy(req *client.BreakglassRequest) string {
	if req == nil {
		return ""
	}
	return req.DecidedBy
}

// breakglassDecidedSummary is decision time + actor for get detail (empty if undecided).
func breakglassDecidedSummary(req *client.BreakglassRequest) string {
	at := breakglassDecidedAt(req)
	if at == "" {
		return ""
	}
	ts := formatBreakglassTime(at)
	by := breakglassDecidedBy(req)
	if by != "" {
		return fmt.Sprintf("%s · %s", ts, by)
	}
	return ts
}

func breakglassActivatesSummary(req *client.BreakglassRequest) string {
	if req == nil || len(req.ActivateAt) == 0 {
		return "-"
	}
	if len(req.ActivateAt) == 1 {
		return formatBreakglassTime(req.ActivateAt[0])
	}
	return fmt.Sprintf("%d activates", len(req.ActivateAt))
}

func breakglassSessionDisplay(sessionID string, wide bool) string {
	if sessionID == "" {
		return "-"
	}
	if wide || len(sessionID) <= 12 {
		return sessionID
	}
	return sessionID[:8]
}

func breakglassSignerShort(arn string) string {
	if arn == "" {
		return "-"
	}
	if i := strings.LastIndex(arn, "/"); i >= 0 && i < len(arn)-1 {
		return arn[i+1:]
	}
	return arn
}

func breakglassValidUntil(req *client.BreakglassRequest) string {
	if req == nil {
		return ""
	}
	switch req.Status {
	case "pending":
		return req.RequestExpiresAt
	case "approved":
		return req.GrantExpiresAt
	default:
		return ""
	}
}

func breakglassTimeLeftShort(until string) string {
	if until == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, until)
	if err != nil {
		t, err = time.Parse(time.RFC3339, until)
	}
	if err != nil {
		return ""
	}
	d := time.Until(t)
	if d < 0 {
		return "ended"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	}
	h := int((d + time.Hour - time.Nanosecond) / time.Hour)
	return fmt.Sprintf("%dh", h)
}
