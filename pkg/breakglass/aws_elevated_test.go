package breakglass

import "testing"

func TestAwsElevatedScope_WhenRegistered_ItShouldMatchCatalogContract(t *testing.T) {
	def, ok := Get("aws-elevated")
	if !ok {
		t.Fatal("aws-elevated not registered")
	}
	meta := def.Metadata
	if meta.Plane != PlaneAWS || meta.Type != TypeElevated {
		t.Fatalf("unexpected plane/type: %s %s", meta.Plane, meta.Type)
	}
	if meta.AwsRoleKey != AwsRoleKeyElevated {
		t.Fatalf("aws_role_key: want %q, got %q", AwsRoleKeyElevated, meta.AwsRoleKey)
	}
	if meta.Authorization.DefaultGrantHours > 2 {
		t.Fatalf("elevated grant hours must be <= 2, got %d", meta.Authorization.DefaultGrantHours)
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
