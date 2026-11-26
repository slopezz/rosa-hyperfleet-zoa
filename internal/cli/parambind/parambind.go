// Package parambind maps TA parameters to zoa run CLI flags and documents global run/request flags.
package parambind

import (
	"fmt"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/reason"
	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
	pkgreason "github.com/openshift-online/rosa-seaplane-zoa/pkg/reason"
)

// RunModifier documents a global zoa run flag (not a TA parameter).
type RunModifier struct {
	Flags       string `json:"flags"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

// ParamBinding maps a TA parameter key to how it is set on the CLI.
type ParamBinding struct {
	Param       string `json:"param"`
	CLI         string `json:"cli"`
	ViaParam    bool   `json:"via_param,omitempty"`
	Description string `json:"description,omitempty"`
}

// DescribeCLI is attached to describe JSON and documents invocation.
type DescribeCLI struct {
	RunModifiers []RunModifier  `json:"run_modifiers"`
	Parameters   []ParamBinding `json:"parameters"`
}

// DescribeView is the enriched describe payload (human + JSON).
type DescribeView struct {
	client.Action
	CLI DescribeCLI `json:"cli"`
}

// ProfileRequestCLI documents zoa breakglass request invocation (parallels DescribeCLI for TAs).
type ProfileRequestCLI struct {
	RequestModifiers []RunModifier `json:"request_modifiers"`
}

// ProfileDescribeView is enriched break-glass profile metadata for describe (-o json).
type ProfileDescribeView struct {
	client.ProfileMetadata `json:",inline"`
	CLI                    ProfileRequestCLI `json:"cli"`
}

// runFlagDocs is the single source of truth for zoa run global flags (cobra help + describe).
type runFlagDoc struct {
	name        string // cobra flag name
	display     string // shown in describe RUN section
	description string
	required    bool
}

const reasonFlagDescription = "Jira issue or PagerDuty incident (required unless " + reason.EnvVar + " is set)"

var runFlagDocs = []runFlagDoc{
	{name: "reason", display: "--reason", description: fmt.Sprintf("%s (%s)", reasonFlagDescription, pkgreason.FormatHint), required: true},
	{name: "force", display: "--force", description: "Bypass write cooldown and concurrency limits (write TAs)"},
	{name: "dry-run", display: "--dry-run", description: "Run the TA's dry_run_action instead of the write TA"},
	{name: "no-wait", display: "--no-wait", description: "Print execution id and exit without TA output (sync: fire-and-forget; async: default unless --wait)"},
	{name: "wait", display: "--wait", description: "Wait for completion and print TA output (async TAs only; sync always waits unless --no-wait)"},
	{name: "timeout", display: "--timeout", description: "Server-side TA execution timeout (e.g. 60s, 3m; bounded by server max 295s)"},
	{name: "execution-mode", display: "--execution-mode", description: "Override execution class: sync or async (default: TA's declared class)"},
	{name: "param", display: "--param key=value", description: "Set any TA parameter by API name (repeatable); used when no dedicated flag exists"},
}

// requestFlagDocs is the single source of truth for zoa breakglass request flags (cobra help + describe).
var requestFlagDocs = []runFlagDoc{
	{name: "reason", display: "--reason", description: runFlagDocs[0].description, required: true},
	{name: "wait", display: "--wait", description: "Wait for approval decision (poll until approved/denied/expired; does not activate credentials)"},
	{name: "wait-timeout", display: "--wait-timeout", description: "Max poll duration when --wait is active (default 5m)"},
	{name: "wait-poll-interval", display: "--wait-poll-interval", description: "Poll frequency when --wait is active (default 30s)"},
}

// RunFlagHelp returns cobra flag help for a documented global run flag.
func RunFlagHelp(flagName string) string {
	for _, d := range runFlagDocs {
		if d.name == flagName {
			return d.description
		}
	}
	return ""
}

// RequestFlagHelp returns cobra flag help for a documented zoa breakglass request flag.
func RequestFlagHelp(flagName string) string {
	for _, d := range requestFlagDocs {
		if d.name == flagName {
			return d.description
		}
	}
	return ""
}

// RunModifiers lists flags that apply to every zoa run invocation.
func RunModifiers() []RunModifier {
	out := make([]RunModifier, 0, len(runFlagDocs))
	for _, d := range runFlagDocs {
		out = append(out, RunModifier{
			Flags:       d.display,
			Description: d.description,
			Required:    d.required,
		})
	}
	return out
}

// RequestModifiers lists flags that apply to every zoa breakglass request invocation.
func RequestModifiers() []RunModifier {
	out := make([]RunModifier, 0, len(requestFlagDocs))
	for _, d := range requestFlagDocs {
		out = append(out, RunModifier{
			Flags:       d.display,
			Description: d.description,
			Required:    d.required,
		})
	}
	return out
}

// EnrichProfileDescribe merges profile metadata with break-glass request CLI hints.
func EnrichProfileDescribe(p *client.ProfileMetadata) ProfileDescribeView {
	view := ProfileDescribeView{}
	if p != nil {
		view.ProfileMetadata = *p
	}
	view.CLI.RequestModifiers = RequestModifiers()
	return view
}

// CLILineForParam returns how to set param on the CLI for describe output.
func CLILineForParam(param string) string {
	if line := describeCLILine(param); line != "" {
		return line
	}
	return fmt.Sprintf("--param %s=...", param)
}

// BindingForParam returns structured binding for JSON describe.
func BindingForParam(param client.ActionParam) ParamBinding {
	if line := describeCLILine(param.Name); line != "" {
		return ParamBinding{
			Param:       param.Name,
			CLI:         line,
			Description: strings.TrimSpace(param.Description),
		}
	}
	return ParamBinding{
		Param:       param.Name,
		CLI:         fmt.Sprintf("--param %s=", param.Name),
		ViaParam:    true,
		Description: strings.TrimSpace(param.Description),
	}
}

// EnrichDescribe merges API action metadata with CLI binding hints.
func EnrichDescribe(action *client.Action) DescribeView {
	view := DescribeView{Action: *action}
	view.CLI.RunModifiers = RunModifiers()
	for _, p := range action.Params {
		view.CLI.Parameters = append(view.CLI.Parameters, BindingForParam(p))
	}
	return view
}
