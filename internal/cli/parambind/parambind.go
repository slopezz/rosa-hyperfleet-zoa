// Package parambind maps Trusted Action parameter names to zoa run CLI flags.
package parambind

import (
	"fmt"
	"strings"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
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

// runFlagDocs is the single source of truth for zoa run global flags (cobra help + describe).
type runFlagDoc struct {
	name        string // cobra flag name
	display     string // shown in describe RUN section
	description string
	required    bool
}

var runFlagDocs = []runFlagDoc{
	{name: "jira", display: "--jira", description: "Jira ticket (required unless env ZOA_JIRA is set)", required: true},
	{name: "force", display: "--force", description: "Bypass write cooldown and concurrency limits (write TAs)"},
	{name: "dry-run", display: "--dry-run", description: "Run the TA's dry_run_action instead of the write TA"},
	{name: "no-wait", display: "--no-wait", description: "Print execution id and exit without TA output (sync: fire-and-forget; async: default unless --wait)"},
	{name: "wait", display: "--wait", description: "Wait for completion and print TA output (async TAs only; sync always waits unless --no-wait)"},
	{name: "timeout", display: "--timeout", description: "Server-side TA execution timeout (e.g. 60s, 3m; bounded by server max 295s)"},
	{name: "execution-mode", display: "--execution-mode", description: "Override execution class: sync or async (default: TA's declared class)"},
	{name: "param", display: "--param key=value", description: "Set any TA parameter by API name (repeatable); used when no dedicated flag exists"},
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
