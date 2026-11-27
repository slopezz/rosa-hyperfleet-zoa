package breakglassactivegrants

import (
	"os"
	"testing"
)

func TestSetGrant_WhenKubeThenAWS_ItShouldKeepBothScopes(t *testing.T) {
	home := t.TempDir()
	kube := GrantStanza{RequestID: "k1", Profile: "kube_read", GrantExpiresAt: "2099-01-01T00:00:00Z"}
	aws := GrantStanza{RequestID: "a1", Profile: "aws_write", GrantExpiresAt: "2099-01-02T00:00:00Z"}
	if err := SetGrant(home, kube); err != nil {
		t.Fatal(err)
	}
	if err := SetGrant(home, aws); err != nil {
		t.Fatal(err)
	}
	f, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if f.Kube == nil || f.Kube.Profile != "kube_read" || f.Kube.RequestID != "k1" {
		t.Fatalf("kube stanza: %+v", f.Kube)
	}
	if f.AWS == nil || f.AWS.Profile != "aws_write" || f.AWS.RequestID != "a1" {
		t.Fatalf("aws stanza: %+v", f.AWS)
	}
}

func TestSetGrant_WhenSecondKubeProfile_ItShouldReplaceKubeStanza(t *testing.T) {
	home := t.TempDir()
	if err := SetGrant(home, GrantStanza{RequestID: "r1", Profile: "kube_read", GrantExpiresAt: "2099-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := SetGrant(home, GrantStanza{RequestID: "r2", Profile: "kube_write", GrantExpiresAt: "2099-01-02T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	f, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if f.Kube.RequestID != "r2" || f.Kube.Profile != "kube_write" {
		t.Fatalf("expected kube_write on r2, got %+v", f.Kube)
	}
	if f.AWS != nil {
		t.Fatalf("expected no aws stanza, got %+v", f.AWS)
	}
}

func TestRead_WhenActiveFileIsNotJSON_ItShouldReturnError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(ConfigDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ActiveFilePath(home), []byte("550e8400-e29b-41d4-a716-446655440000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(home); err == nil {
		t.Fatal("expected error for non-JSON active file")
	}
}
