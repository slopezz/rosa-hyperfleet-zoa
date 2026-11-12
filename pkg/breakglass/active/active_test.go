package active

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivate_WhenKubeThenAWS_ItShouldKeepBothStanzas(t *testing.T) {
	home := t.TempDir()
	kube := Stanza{RequestID: "k1", Scope: "kube-read", GrantExpiresAt: "2099-01-01T00:00:00Z"}
	aws := Stanza{RequestID: "a1", Scope: "aws-write", GrantExpiresAt: "2099-01-02T00:00:00Z"}
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
	if a.Kube == nil || a.Kube.Scope != "kube-read" {
		t.Fatalf("kube stanza: %+v", a.Kube)
	}
	if a.AWS == nil || a.AWS.Scope != "aws-write" {
		t.Fatalf("aws stanza: %+v", a.AWS)
	}
}

func TestActivate_WhenSecondKubeUse_ItShouldReplaceKubeStanza(t *testing.T) {
	home := t.TempDir()
	if err := Activate(home, Stanza{RequestID: "r1", Scope: "kube-read", GrantExpiresAt: "2099-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := Activate(home, Stanza{RequestID: "r2", Scope: "kube-write", GrantExpiresAt: "2099-01-02T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube.RequestID != "r2" || a.Kube.Scope != "kube-write" {
		t.Fatalf("expected kube-write on r2, got %+v", a.Kube)
	}
}

func TestRead_WhenLegacySingleLine_ItShouldMigrateFromMeta(t *testing.T) {
	home := t.TempDir()
	dir := RequestDir(home, "550e8400-e29b-41d4-a716-446655440000")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	meta := `{"request_id":"550e8400-e29b-41d4-a716-446655440000","scope":"kube-read","grant_expires_at":"2099-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ActivePath(home), []byte("550e8400-e29b-41d4-a716-446655440000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube == nil || a.Kube.Scope != "kube-read" {
		t.Fatalf("migrate: %+v", a)
	}
	raw, _ := os.ReadFile(ActivePath(home))
	if raw[0] != '{' {
		t.Fatal("expected JSON active file after migrate")
	}
}
