package active

import (
	"os"
	"testing"
)

func TestActivate_WhenKubeThenAWS_ItShouldKeepBothStanzas(t *testing.T) {
	home := t.TempDir()
	kube := Stanza{RequestID: "k1", Name: "kube_read", GrantExpiresAt: "2099-01-01T00:00:00Z"}
	aws := Stanza{RequestID: "a1", Name: "aws_write", GrantExpiresAt: "2099-01-02T00:00:00Z"}
	if err := Activate(home, kube); err != nil {
		t.Fatal(err)
	}
	if err := Activate(home, aws); err != nil {
		t.Fatal(err)
	}
	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube == nil || a.Kube.Name != "kube_read" {
		t.Fatalf("kube stanza: %+v", a.Kube)
	}
	if a.AWS == nil || a.AWS.Name != "aws_write" {
		t.Fatalf("aws stanza: %+v", a.AWS)
	}
}

func TestActivate_WhenSecondKubeUse_ItShouldReplaceKubeStanza(t *testing.T) {
	home := t.TempDir()
	if err := Activate(home, Stanza{RequestID: "r1", Name: "kube_read", GrantExpiresAt: "2099-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := Activate(home, Stanza{RequestID: "r2", Name: "kube_write", GrantExpiresAt: "2099-01-02T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube.RequestID != "r2" || a.Kube.Name != "kube_write" {
		t.Fatalf("expected kube_write on r2, got %+v", a.Kube)
	}
}

func TestRead_WhenActiveIsNotJSON_ItShouldReturnError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(ConfigDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ActivePath(home), []byte("550e8400-e29b-41d4-a716-446655440000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(home); err == nil {
		t.Fatal("expected error for non-JSON active file")
	}
}
