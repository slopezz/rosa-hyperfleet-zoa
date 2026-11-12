package scopes

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestConformance_WhenAllScopesRegistered_ItShouldHaveValidAuthorization(t *testing.T) {
	actions.SetTargetType(actions.TargetTypeRC)
	all := List()
	if len(all) < 6 {
		t.Fatalf("expected at least 6 scopes, got %d", len(all))
	}
	for _, meta := range all {
		switch meta.Type {
		case TypeElevated:
			if meta.Authorization.Approval != "strict" {
				t.Errorf("scope %s elevated must use strict approval", meta.ID)
			}
			if meta.Authorization.DefaultGrantHours > 2 {
				t.Errorf("scope %s elevated grant hours must be <= 2", meta.ID)
			}
		case TypeRead, TypeWrite:
			if meta.Authorization.DefaultGrantHours <= 0 {
				t.Errorf("scope %s must set default grant hours", meta.ID)
			}
		}
	}
}
