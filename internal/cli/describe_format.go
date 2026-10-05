package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func printDescribeHuman(action *client.Action, w io.Writer) {
	view := parambind.EnrichDescribe(action)

	fmt.Fprintf(w, "NAME:        %s\n", view.Name)
	fmt.Fprintf(w, "SCOPE:       %s\n", view.Scope)
	fmt.Fprintf(w, "TYPE:        %s\n", view.Type)
	fmt.Fprintf(w, "MODE:        %s\n", output.Dash(view.ExecutionMode))
	fmt.Fprintf(w, "DESCRIPTION: %s\n", view.Description)
	fmt.Fprintf(w, "APPROVAL:    %s\n", output.Dash(view.Authorization.Approval))

	if view.WriteCooldownSeconds > 0 {
		fmt.Fprintf(w, "COOLDOWN:    %ds\n", view.WriteCooldownSeconds)
	}
	if view.TimeoutSeconds > 0 {
		fmt.Fprintf(w, "TIMEOUT:     %ds\n", view.TimeoutSeconds)
	}
	if view.DryRunAction != "" {
		fmt.Fprintf(w, "DRY-RUN:     %s\n", view.DryRunAction)
	}

	fmt.Fprintf(w, "\nRUN (global flags on every zoa run):\n")
	for _, m := range view.CLI.RunModifiers {
		req := ""
		if m.Required {
			req = " (required)"
		}
		fmt.Fprintf(w, "  %-28s %s%s\n", m.Flags, m.Description, req)
	}

	if len(view.Params) > 0 {
		fmt.Fprintf(w, "\nPARAMETERS (API name → how to pass on CLI):\n")
		tw := output.NewTable(w)
		fmt.Fprintf(tw, "  NAME\tREQUIRED\tDEFAULT\tCLI\tDESCRIPTION\n")
		for _, p := range view.Params {
			req := ""
			if p.Required {
				req = "*"
			}
			cli := parambind.CLILineForParam(p.Name)
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
				p.Name, req, output.Dash(p.Default), cli, strings.TrimSpace(p.Description))
		}
		tw.Flush()
	} else {
		fmt.Fprintf(w, "\nPARAMETERS:  (none — only --jira and run modifiers apply)\n")
	}

	if len(view.Examples) > 0 {
		fmt.Fprintf(w, "\nEXAMPLES:\n")
		for _, ex := range view.Examples {
			fmt.Fprintf(w, "  %s\n", ex)
		}
	}
}
