package breakglass

import (
	"errors"
	"fmt"
	"time"
)

// KubeCredentialMaxSeconds is ZOA's per-activate cap (TokenRequest allows longer; see authentication.k8s.io TokenRequest validation).
const KubeCredentialMaxSeconds int64 = 3600

// AWSCredentialMaxSeconds matches STS AssumeRole default session length; API allows up to role MaxSessionDuration unless ZOA caps lower.
const AWSCredentialMaxSeconds int64 = 3600

// MinKubeCredentialSeconds is the K8s TokenRequest minimum (kubernetes/apiserver validation: 10 minutes).
const MinKubeCredentialSeconds int64 = 600

// MinAWSCredentialSeconds is the STS AssumeRole DurationSeconds minimum (AWS STS API Reference: 900 seconds).
const MinAWSCredentialSeconds int64 = 900

// ErrGrantTooShortForCredential is returned when remaining grant time is below the cloud API minimum credential lifetime.
var ErrGrantTooShortForCredential = errors.New("breakglass: grant remaining below minimum credential lifetime")

// GrantTooShortError details activate rejection when grant remaining is shorter than Min*CredentialSeconds.
type GrantTooShortError struct {
	Scope         string
	Remaining     time.Duration
	MinCredential time.Duration
	MaxCredential time.Duration
}

func (e *GrantTooShortError) Error() string {
	return fmt.Sprintf(
		"remaining grant %s is less than minimum %s credential lifetime %s",
		formatCredentialDuration(e.Remaining),
		e.Scope,
		formatCredentialDuration(e.MinCredential),
	)
}

func (e *GrantTooShortError) Unwrap() error { return ErrGrantTooShortForCredential }

// CredentialMaxSeconds returns the platform max credential lifetime for activate on the given scope (kube or aws).
func CredentialMaxSeconds(scope string) int64 {
	switch scope {
	case ScopeAWS:
		return AWSCredentialMaxSeconds
	case ScopeKube:
		return KubeCredentialMaxSeconds
	default:
		return KubeCredentialMaxSeconds
	}
}

// CredentialMinSeconds returns the cloud API minimum credential lifetime for the scope (kube or aws).
func CredentialMinSeconds(scope string) int64 {
	switch scope {
	case ScopeAWS:
		return MinAWSCredentialSeconds
	case ScopeKube:
		return MinKubeCredentialSeconds
	default:
		return MinKubeCredentialSeconds
	}
}

// CredentialMaxDuration returns the platform max credential lifetime as a time.Duration.
func CredentialMaxDuration(scope string) time.Duration {
	return time.Duration(CredentialMaxSeconds(scope)) * time.Second
}

// CredentialMaxTTLDisplay formats the platform max for CLI/catalog (e.g. "1h").
func CredentialMaxTTLDisplay(scope string) string {
	return formatCredentialDuration(CredentialMaxDuration(scope))
}

// CredentialDurationForGrant returns how long to ask STS/TokenRequest for: min(remaining, scope max), or GrantTooShortError if below API min.
func CredentialDurationForGrant(scope string, grantRemaining time.Duration) (time.Duration, error) {
	if grantRemaining <= 0 {
		return 0, &GrantTooShortError{
			Scope:         scope,
			Remaining:     grantRemaining,
			MinCredential: time.Duration(CredentialMinSeconds(scope)) * time.Second,
			MaxCredential: CredentialMaxDuration(scope),
		}
	}
	secs := int64(grantRemaining.Seconds())
	max := CredentialMaxSeconds(scope)
	if secs > max {
		secs = max
	}
	min := CredentialMinSeconds(scope)
	if secs < min {
		return 0, &GrantTooShortError{
			Scope:         scope,
			Remaining:     grantRemaining,
			MinCredential: time.Duration(min) * time.Second,
			MaxCredential: time.Duration(max) * time.Second,
		}
	}
	return time.Duration(secs) * time.Second, nil
}

// ActivateGrantTooShortMessage is the user-facing API/CLI reason for HTTP 409 grant_too_short.
func ActivateGrantTooShortMessage(scope string, grantRemaining time.Duration) string {
	min := time.Duration(CredentialMinSeconds(scope)) * time.Second
	return fmt.Sprintf(
		"remaining grant time (%s) is below the minimum %s break-glass credential lifetime (%s); submit a new break-glass request",
		formatCredentialDuration(grantRemaining),
		scope,
		formatCredentialDuration(min),
	)
}

func formatCredentialDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	}
	h := int(d.Round(time.Minute) / time.Hour)
	m := int(d.Round(time.Minute)/time.Minute) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}