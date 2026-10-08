// Package reason resolves the operator reason for zoa run (--reason flag, then ZOA_REASON).
package reason

import (
	"fmt"
	"os"

	pkgreason "github.com/openshift-online/rosa-hyperfleet-zoa/pkg/reason"
)

// EnvVar is the session default set by the boundary task env and reason() shell helper.
const EnvVar = "ZOA_REASON"

// FormatHint re-exports accepted reason formats for flag help (matches API error text).
const FormatHint = pkgreason.FormatHint

// Resolve returns the reason: non-empty flag value, then EnvVar, else error.
func Resolve(flagValue string) (string, error) {
	if v, err := pkgreason.Normalize(flagValue); err != nil {
		return "", err
	} else if v != "" {
		return v, nil
	}
	if v, err := pkgreason.Normalize(os.Getenv(EnvVar)); err != nil {
		return "", err
	} else if v != "" {
		return v, nil
	}
	return "", fmt.Errorf("reason is required: use --reason or %s (%s)", EnvVar, pkgreason.FormatHint)
}
