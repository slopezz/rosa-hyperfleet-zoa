package jira

import (
	"testing"
)

func TestResolve_WhenFlagSet_ItShouldPreferFlagOverEnv(t *testing.T) {
	t.Setenv(EnvVar, "ROSAENG-1")
	j, err := Resolve("ROSAENG-999")
	if err != nil || j != "ROSAENG-999" {
		t.Fatalf("got %q err=%v", j, err)
	}
}

func TestResolve_WhenEnvSetWithoutFlag_ItShouldUseEnv(t *testing.T) {
	t.Setenv(EnvVar, "ROSAENG-200")
	j, err := Resolve("")
	if err != nil || j != "ROSAENG-200" {
		t.Fatalf("got %q err=%v", j, err)
	}
}

func TestResolve_WhenNeitherSet_ItShouldError(t *testing.T) {
	t.Setenv(EnvVar, "")
	_, err := Resolve("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolve_WhenInvalidFlag_ItShouldErrorDespiteEnv(t *testing.T) {
	t.Setenv(EnvVar, "ROSAENG-1")
	_, err := Resolve("bad")
	if err == nil {
		t.Fatal("expected invalid flag error")
	}
}

func TestResolve_WhenInvalidEnv_ItShouldError(t *testing.T) {
	t.Setenv(EnvVar, "not-valid")
	_, err := Resolve("")
	if err == nil {
		t.Fatal("expected invalid env error")
	}
}

func TestRequireFlag_WhenValid_ItShouldReturnTicket(t *testing.T) {
	j, err := RequireFlag("ROSAENG-42")
	if err != nil || j != "ROSAENG-42" {
		t.Fatalf("got %q err=%v", j, err)
	}
}

func TestRequireFlag_WhenEmpty_ItShouldError(t *testing.T) {
	_, err := RequireFlag("")
	if err == nil {
		t.Fatal("expected error")
	}
}
