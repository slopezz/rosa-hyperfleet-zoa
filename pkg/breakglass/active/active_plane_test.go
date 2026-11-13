package active

import (
	"path/filepath"
	"testing"
)

func TestActivate_WhenKubeWriteAfterKubeRead_ItShouldReplaceKubeStanza(t *testing.T) {
	home := t.TempDir()
	_ = Activate(home, Stanza{RequestID: "read-req", Scope: "kube-read", GrantExpiresAt: "2099-01-01T00:00:00Z"})
	_ = Activate(home, Stanza{RequestID: "write-req", Scope: "kube-write", GrantExpiresAt: "2099-01-02T00:00:00Z"})

	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube == nil || a.Kube.RequestID != "write-req" || a.Kube.Scope != "kube-write" {
		t.Fatalf("kube stanza: %+v", a.Kube)
	}
	if a.AWS != nil {
		t.Fatalf("expected no aws stanza, got %+v", a.AWS)
	}
	if _, err := filepath.Abs(ActivePath(home)); err != nil {
		t.Fatal(err)
	}
}

func TestActivate_WhenKubeAndAWS_ItShouldKeepBothPlanes(t *testing.T) {
	home := t.TempDir()
	_ = Activate(home, Stanza{RequestID: "k1", Scope: "kube-read"})
	_ = Activate(home, Stanza{RequestID: "a1", Scope: "aws-read"})

	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube == nil || a.AWS == nil {
		t.Fatalf("expected both planes: %+v", a)
	}
}
