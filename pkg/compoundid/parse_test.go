package compoundid

import "testing"

func TestParse_WhenValidCompound_ItShouldSplitDeploymentAndID(t *testing.T) {
	dep, id, err := Parse("eph-us-east-1/550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep != "eph-us-east-1" || id != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("got dep=%q id=%q", dep, id)
	}
}

func TestParse_WhenInvalid_ItShouldError(t *testing.T) {
	if _, _, err := Parse("noseparator"); err == nil {
		t.Fatal("expected error")
	}
}
