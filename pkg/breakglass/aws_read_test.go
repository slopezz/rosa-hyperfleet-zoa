package breakglass

import "testing"

func TestAwsReadScope_WhenRegistered_ItShouldMatchCatalogContract(t *testing.T) {
	def, ok := Get("aws_read")
	if !ok {
		t.Fatal("aws_read not registered")
	}
	meta := def.Metadata
	if meta.Scope != ScopeAWS || meta.Type != TypeRead {
		t.Fatalf("unexpected plane/type: %s %s", meta.Scope, meta.Type)
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
