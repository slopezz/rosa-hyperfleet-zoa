// Package jiraticket validates Jira issue keys used on boundary sessions and TA dispatch.
package jiraticket

import (
	"fmt"
	"regexp"
	"strings"
)

var ticketPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

// Normalize trims and validates a Jira key (e.g. ROSAENG-1234). Empty input is allowed.
func Normalize(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !ticketPattern.MatchString(s) {
		return "", fmt.Errorf("invalid jira %q: must match PROJECT-123 (e.g. ROSAENG-1234)", s)
	}
	return s, nil
}

// Require is like Normalize but errors when empty.
func Require(s string) (string, error) {
	n, err := Normalize(s)
	if err != nil {
		return "", err
	}
	if n == "" {
		return "", fmt.Errorf("missing jira ticket")
	}
	return n, nil
}
