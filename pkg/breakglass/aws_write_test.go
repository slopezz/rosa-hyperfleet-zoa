package breakglass

import "testing"

func TestAwsWriteScope_WhenRegistered_ItShouldMatchCatalogContract(t *testing.T) {
	def, ok := Get("aws-write")
	if !ok {
		t.Fatal("aws-write not registered")
	}
	meta := def.Metadata
	if meta.Plane != PlaneAWS || meta.Type != TypeWrite {
		t.Fatalf("unexpected plane/type: %s %s", meta.Plane, meta.Type)
	}
	if meta.AwsRoleKey != AwsRoleKeyWrite {
		t.Fatalf("aws_role_key: want %q, got %q", AwsRoleKeyWrite, meta.AwsRoleKey)
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
