package targettype

import (
	"testing"
)

func TestResolve_WhenFlagSet_ItShouldPreferFlag(t *testing.T) {
	t.Setenv(EnvVar, "mc")
	t.Setenv(LegacyEnvVar, "mc")
	got, err := Resolve("rc")
	if err != nil {
		t.Fatal(err)
	}
	if got != RC {
		t.Fatalf("got %q, want rc", got)
	}
}

func TestResolve_WhenOnlyEnvVarSet_ItShouldUseEnvVar(t *testing.T) {
	t.Setenv(EnvVar, "mc")
	t.Setenv(LegacyEnvVar, "")
	got, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got != MC {
		t.Fatalf("got %q, want mc", got)
	}
}

func TestResolve_WhenLegacyEnvVarSet_ItShouldUseLegacy(t *testing.T) {
	t.Setenv(EnvVar, "")
	t.Setenv(LegacyEnvVar, "rc")
	got, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got != RC {
		t.Fatalf("got %q, want rc", got)
	}
}

func TestResolve_WhenUnset_ItShouldReturnError(t *testing.T) {
	t.Setenv(EnvVar, "")
	t.Setenv(LegacyEnvVar, "")
	_, err := Resolve("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolve_WhenInvalidValue_ItShouldReturnError(t *testing.T) {
	_, err := Resolve("hcp")
	if err == nil {
		t.Fatal("expected error")
	}
}
