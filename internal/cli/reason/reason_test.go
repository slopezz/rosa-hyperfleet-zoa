package reason

import (
	"testing"
)

func TestResolve_WhenFlagSet_ItShouldPreferFlagOverEnv(t *testing.T) {
	t.Setenv(EnvVar, "ROSAENG-1")
	r, err := Resolve("ROSAENG-999")
	if err != nil || r != "ROSAENG-999" {
		t.Fatalf("got %q err=%v", r, err)
	}
}

func TestResolve_WhenPagerDutyInEnv_ItShouldUseEnv(t *testing.T) {
	t.Setenv(EnvVar, "#123456")
	r, err := Resolve("")
	if err != nil || r != "#123456" {
		t.Fatalf("got %q err=%v", r, err)
	}
}

func TestResolve_WhenFlagInvalid_ItShouldNotFallBackToEnv(t *testing.T) {
	t.Setenv(EnvVar, "ROSAENG-1")
	_, err := Resolve("not-valid")
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
}
