// Package jira resolves the Jira ticket for zoa run (--jira flag, then ZOA_JIRA).
package jira

import (
	"fmt"
	"os"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/jiraticket"
)

// EnvVar is the session default set by the boundary task env and jira() shell helper.
const EnvVar = "ZOA_JIRA"

// Resolve returns the ticket: non-empty flag value, then EnvVar, else error.
func Resolve(flagValue string) (string, error) {
	if v, err := jiraticket.Normalize(flagValue); err != nil {
		return "", err
	} else if v != "" {
		return v, nil
	}
	if v, err := jiraticket.Normalize(os.Getenv(EnvVar)); err != nil {
		return "", err
	} else if v != "" {
		return v, nil
	}
	return "", fmt.Errorf("missing jira: pass --jira or set %s (boundary: set at session start or jira ROSAENG-1234)", EnvVar)
}

// RequireFlag validates a required --jira value (session start).
func RequireFlag(flagValue string) (string, error) {
	ticket, err := jiraticket.Require(flagValue)
	if err != nil {
		return "", fmt.Errorf("missing or invalid --jira: %w (e.g. ROSAENG-1234)", err)
	}
	return ticket, nil
}
