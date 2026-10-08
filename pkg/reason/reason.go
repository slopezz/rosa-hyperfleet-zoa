// Package reason validates operator reason strings on boundary sessions and TA dispatch.
package reason

import (
	"fmt"
	"regexp"
	"strings"
)

var jiraPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)
var pagerDutyIncidentPattern = regexp.MustCompile(`^#[0-9]+$`)

// ExampleJiraIssue and ExamplePagerDutyIncident are fictional placeholders (not real tickets).
const (
	ExampleJiraIssue         = "ROSAENG-1234"
	ExamplePagerDutyIncident = "#123456"
)

// FormatHint describes accepted shapes in API/CLI errors (no "example" label — IDs are clearly fictional).
const FormatHint = "Jira issue " + ExampleJiraIssue + " or PagerDuty incident " + ExamplePagerDutyIncident

func errInvalidReason(value string) error {
	return fmt.Errorf("reason %q is not valid (%s)", value, FormatHint)
}

func errMissingReason() error {
	return fmt.Errorf("reason is required (%s)", FormatHint)
}

// Normalize trims and validates reason (Jira issue or PagerDuty incident). Empty input is allowed.
func Normalize(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if jiraPattern.MatchString(s) || pagerDutyIncidentPattern.MatchString(s) {
		return s, nil
	}
	return "", errInvalidReason(s)
}

// Require is like Normalize but errors when empty.
func Require(s string) (string, error) {
	n, err := Normalize(s)
	if err != nil {
		return "", err
	}
	if n == "" {
		return "", errMissingReason()
	}
	return n, nil
}
