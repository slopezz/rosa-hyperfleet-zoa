package scopecatalog

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

func TestList_WhenTargetTypeRC_ItShouldReturnKubeScopes(t *testing.T) {
	items, err := List(actions.TargetTypeRC)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 3 {
		t.Fatalf("expected kube scopes on rc, got %d", len(items))
	}
	found := false
	for _, s := range items {
		if s.ID == "kube-read" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected kube-read on rc")
	}
}

func TestGet_WhenScopeMissing_ItShouldError(t *testing.T) {
	_, err := Get(actions.TargetTypeRC, "not-a-scope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientScopeFromMetadata_WhenApprovalSet_ItShouldMapAuthorization(t *testing.T) {
	meta := breakglass.ScopeMetadata{
		ID:    "kube-read",
		Plane: breakglass.PlaneKube,
		Type:  breakglass.TypeRead,
		Authorization: breakglass.Authorization{
			Approval:            "none",
			DefaultGrantHours:   4,
			DefaultPendingHours: 24,
		},
	}
	out := ClientScopeFromMetadata(meta)
	if out.Authorization["approval"] != "none" {
		t.Fatalf("approval: %v", out.Authorization["approval"])
	}
}
