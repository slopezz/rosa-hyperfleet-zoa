package api

import "testing"

func TestParseCompoundResourceID_WhenValid_ItShouldSplit(t *testing.T) {
	dep, id, err := ParseCompoundResourceID("us-east-1-eph/550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep != "us-east-1-eph" || id != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected split: %q %q", dep, id)
	}
}

func TestParseCompoundResourceID_WhenInvalid_ItShouldError(t *testing.T) {
	_, _, err := ParseCompoundResourceID("noseparator")
	if err == nil {
		t.Fatal("expected error")
	}
}
