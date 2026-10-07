// Package jira resolves the Jira ticket for zoa run (--jira flag, then ZOA_JIRA).
package jira

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// EnvVar is the session default set by the boundary jira() shell helper.
const EnvVar = "ZOA_JIRA"

var ticketPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// Resolve returns the ticket: non-empty flag value, then EnvVar, else error.
func Resolve(flagValue string) (string, error) {
	if v, err := normalizeTicket(flagValue); err != nil {
		return "", err
	} else if v != "" {
		return v, nil
	}
	if v, err := normalizeTicket(os.Getenv(EnvVar)); err != nil {
		return "", err
	} else if v != "" {
		return v, nil
	}
	return "", fmt.Errorf("missing jira: pass --jira or set %s (boundary: jira ROSAENG-1234)", EnvVar)
}

func normalizeTicket(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !ticketPattern.MatchString(s) {
		return "", fmt.Errorf("invalid jira %q: must match PROJECT-123 (e.g. ROSAENG-1234)", s)
	}
	return s, nil
}
