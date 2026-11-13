package breakglass

import "testing"

func TestAwsReadScope_WhenRegistered_ItShouldMatchCatalogContract(t *testing.T) {
	def, ok := Get("aws-read")
	if !ok {
		t.Fatal("aws-read not registered")
	}
	meta := def.Metadata
	if meta.Plane != PlaneAWS || meta.Type != TypeRead {
		t.Fatalf("unexpected plane/type: %s %s", meta.Plane, meta.Type)
	}
	if meta.AwsRoleKey != AwsRoleKeyRead {
		t.Fatalf("aws_role_key: want %q, got %q", AwsRoleKeyRead, meta.AwsRoleKey)
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
