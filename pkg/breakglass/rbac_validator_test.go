package breakglass

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestValidateScope_WhenReadScopeHasPatchVerb_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ScopeMetadata{
			ID:          "bad-read",
			Plane:       PlaneKube,
			Type:        TypeRead,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"patch"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected error for write verb on read scope")
	}
}

func TestValidateScope_WhenWriteScopeListsSecrets_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ScopeMetadata{
			ID:          "bad-secrets",
			Plane:       PlaneKube,
			Type:        TypeWrite,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected error for secrets outside elevated scope")
	}
}

func TestValidateScope_WhenElevatedScopeListsSecretRead_ItShouldAllow(t *testing.T) {
	def := &Definition{
		Metadata: ScopeMetadata{
			ID:          "elevated-secrets",
			Plane:       PlaneKube,
			Type:        TypeElevated,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultPendingHours: 8,
			},
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
			}
		},
	}
	if err := ValidateScope(def); err != nil {
		t.Fatalf("expected elevated secret read to be allowed: %v", err)
	}
}
