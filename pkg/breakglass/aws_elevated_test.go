package breakglass

import "testing"

func TestAwsElevatedScope_WhenRegistered_ItShouldMatchCatalogContract(t *testing.T) {
	def, ok := Get("aws_elevated")
	if !ok {
		t.Fatal("aws_elevated not registered")
	}
	meta := def.Metadata
	if meta.Scope != ScopeAWS || meta.Type != TypeElevated {
		t.Fatalf("unexpected scope/type: %s %s", meta.Scope, meta.Type)
	}
	if meta.Authorization.DefaultGrantHours > 2 {
		t.Fatalf("elevated grant hours must be <= 2, got %d", meta.Authorization.DefaultGrantHours)
	}
	if err := ValidateProfile(def); err != nil {
		t.Fatal(err)
	}
}
