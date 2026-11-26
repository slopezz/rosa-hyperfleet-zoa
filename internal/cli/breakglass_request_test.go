package cli

import (
	"strings"
	"testing"
)

func TestBreakglassRequestArgs_WhenMissingProfile_ItShouldErrorWithUsage(t *testing.T) {
	cmd := newBreakglassRequestCommand(&GlobalOptions{})
	err := cmd.Args(cmd, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing profile name") {
		t.Fatalf("got %v", err)
	}
}