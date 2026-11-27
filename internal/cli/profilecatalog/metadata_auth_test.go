package profilecatalog

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestApproval_WhenAuthorizationMissing_ItShouldReturnEmpty(t *testing.T) {
	if Approval(client.ProfileMetadata{}) != "" {
		t.Fatal("expected empty")
	}
}

func TestGrantTTL_WhenHoursPresent_ItShouldFormatDuration(t *testing.T) {
	s := client.ProfileMetadata{Authorization: map[string]interface{}{"default_grant_hours": float64(1)}}
	if GrantTTL(s) != "1h" {
		t.Fatalf("got %q", GrantTTL(s))
	}
}

func TestRequestTTL_WhenHoursPresent_ItShouldFormatDuration(t *testing.T) {
	s := client.ProfileMetadata{Authorization: map[string]interface{}{"default_request_hours": float64(8)}}
	if RequestTTL(s) != "8h" {
		t.Fatalf("got %q", RequestTTL(s))
	}
}

func TestCredentialTTL_WhenScopeSet_ItShouldUsePlatformMax(t *testing.T) {
	s := client.ProfileMetadata{Scope: "kube"}
	if CredentialTTL(s) != "1h" {
		t.Fatalf("got %q", CredentialTTL(s))
	}
}
