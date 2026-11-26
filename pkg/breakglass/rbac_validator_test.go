package breakglass

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestValidateScope_WhenReadScopeHasPatchVerb_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ProfileMetadata{
			Name: "bad-read",
			Scope: ScopeKube,
			Type:        TypeRead,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
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
		Metadata: ProfileMetadata{
			Name: "bad-secrets",
			Scope: ScopeKube,
			Type:        TypeWrite,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
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
		Metadata: ProfileMetadata{
			Name: "elevated-secrets",
			Scope: ScopeKube,
			Type:        TypeElevated,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultRequestHours: 8,
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

func elevatedTestAuth() Authorization {
	return Authorization{
		Approval:            "none",
		DefaultGrantHours:   1,
		DefaultRequestHours: 8,
	}
}

func TestValidateScope_WhenElevatedGrantsDeleteWithStarResources_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ProfileMetadata{
			Name: "bad-elevated-star",
			Scope: ScopeKube,
			Type:          TypeElevated,
			Description:   "test",
			Authorization: elevatedTestAuth(),
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{"hypershift.openshift.io"}, Resources: []string{"*"}, Verbs: []string{"delete", "get"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected reject delete with resources *")
	}
}

func TestValidateScope_WhenElevatedGrantsDeleteOnNamespaces_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ProfileMetadata{
			Name: "bad-elevated-ns",
			Scope: ScopeKube,
			Type:          TypeElevated,
			Description:   "test",
			Authorization: elevatedTestAuth(),
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"delete", "get", "list"}},
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: verbsRead},
				{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected reject delete on namespaces")
	}
}

func TestValidateScope_WhenWriteGrantsPodsExec_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ProfileMetadata{
			Name: "bad-exec",
			Scope: ScopeKube,
			Type:        TypeWrite,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected reject pods/exec outside elevated")
	}
}

func TestValidateScope_WhenElevatedGrantsSecretWrite_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ProfileMetadata{
			Name: "bad-secret-write",
			Scope: ScopeKube,
			Type:          TypeElevated,
			Description:   "test",
			Authorization: elevatedTestAuth(),
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "patch"}},
				{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected reject secret write verbs")
	}
}

func TestValidateScope_WhenKubeScopeMutatesClusterRoleBindings_ItShouldReject(t *testing.T) {
	def := &Definition{
		Metadata: ProfileMetadata{
			Name: "bad-bind",
			Scope: ScopeKube,
			Type:        TypeWrite,
			Description: "test",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
		},
		RBAC: func(string) []actions.RBACRule {
			return []actions.RBACRule{
				{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"clusterrolebindings"}, Verbs: []string{"patch"}},
			}
		},
	}
	if err := ValidateScope(def); err == nil {
		t.Fatal("expected reject RBAC bind mutation")
	}
}

func TestValidateScope_WhenRegisteredKubeScopes_TheyShouldPassValidateScope(t *testing.T) {
	for _, id := range []string{"kube_read", "kube_write", "kube_elevated"} {
		id := id
		t.Run(id, func(t *testing.T) {
			def, ok := Get(id)
			if !ok {
				t.Fatal("not registered")
			}
			if err := ValidateScope(def); err != nil {
				t.Fatal(err)
			}
		})
	}
}
