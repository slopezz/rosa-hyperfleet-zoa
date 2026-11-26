package breakglass

import "testing"

func TestAwsWriteScope_WhenRegistered_ItShouldMatchCatalogContract(t *testing.T) {
	def, ok := Get("aws_write")
	if !ok {
		t.Fatal("aws_write not registered")
	}
	meta := def.Metadata
	if meta.Scope != ScopeAWS || meta.Type != TypeWrite {
		t.Fatalf("unexpected plane/type: %s %s", meta.Scope, meta.Type)
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
