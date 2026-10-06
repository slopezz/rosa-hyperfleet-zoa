// Package targettype resolves ZOA target type (rc or mc) for TA registry filtering.
package targettype

import (
	"fmt"
	"os"
	"strings"
)

const (
	// EnvVar is the canonical environment variable (matches TYPE in `zoa targets`).
	EnvVar = "ZOA_TARGET_TYPE"
	// LegacyEnvVar is deprecated; still read as a fallback during rollout.
	LegacyEnvVar = "ZOA_DEPLOYMENT_TARGET"

	RC = "rc"
	MC = "mc"
)

// Resolve returns normalized target type: flag value, then EnvVar, then LegacyEnvVar.
func Resolve(flagValue string) (string, error) {
	if v := normalize(flagValue); v != "" {
		return v, nil
	}
	if v := normalize(os.Getenv(EnvVar)); v != "" {
		return v, nil
	}
	if v := normalize(os.Getenv(LegacyEnvVar)); v != "" {
		return v, nil
	}
	return "", fmt.Errorf(
		"target type is required (rc or mc): set --target-type, %s, or %s",
		EnvVar, LegacyEnvVar,
	)
}

// ResolveOptional returns normalized target type or empty when unset.
func ResolveOptional(flagValue string) string {
	v, err := Resolve(flagValue)
	if err != nil {
		return ""
	}
	return v
}

func normalize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case RC, MC:
		return s
	default:
		return ""
	}
}
