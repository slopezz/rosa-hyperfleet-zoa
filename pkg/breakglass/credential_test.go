package breakglass

import (
	"errors"
	"testing"
	"time"
)

func TestCredentialDurationForGrant_WhenGrantAboveMin_ItShouldCapAtMax(t *testing.T) {
	got, err := CredentialDurationForGrant(ScopeKube, 2*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != time.Hour {
		t.Fatalf("expected 1h, got %v", got)
	}
}

func TestCredentialDurationForGrant_WhenGrantBetweenMinAndMax_ItShouldUseRemaining(t *testing.T) {
	got, err := CredentialDurationForGrant(ScopeKube, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 30*time.Minute {
		t.Fatalf("expected 30m, got %v", got)
	}
}

func TestCredentialDurationForGrant_WhenGrantBelowKubeMin_ItShouldReturnGrantTooShort(t *testing.T) {
	_, err := CredentialDurationForGrant(ScopeKube, 8*time.Minute)
	if err == nil {
		t.Fatal("expected error")
	}
	var short *GrantTooShortError
	if !errors.As(err, &short) {
		t.Fatalf("expected *GrantTooShortError, got %T: %v", err, err)
	}
	if short.Scope != ScopeKube {
		t.Fatalf("scope = %q", short.Scope)
	}
	if !errors.Is(err, ErrGrantTooShortForCredential) {
		t.Fatalf("expected ErrGrantTooShortForCredential in chain")
	}
}

func TestCredentialDurationForGrant_WhenGrantBelowAWSMin_ItShouldReturnGrantTooShort(t *testing.T) {
	_, err := CredentialDurationForGrant(ScopeAWS, 12*time.Minute)
	if err == nil {
		t.Fatal("expected error")
	}
	var short *GrantTooShortError
	if !errors.As(err, &short) {
		t.Fatalf("expected *GrantTooShortError, got %T", err)
	}
	if short.Scope != ScopeAWS {
		t.Fatalf("scope = %q", short.Scope)
	}
}

func TestCredentialDurationForGrant_WhenGrantExactlyAWSMin_ItShouldSucceed(t *testing.T) {
	got, err := CredentialDurationForGrant(ScopeAWS, 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 15*time.Minute {
		t.Fatalf("expected 15m, got %v", got)
	}
}
