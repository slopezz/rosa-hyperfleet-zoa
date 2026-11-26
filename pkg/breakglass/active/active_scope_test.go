package active

import (
	"path/filepath"
	"testing"
)

func TestActivate_WhenKubeWriteAfterKubeRead_ItShouldReplaceKubeStanza(t *testing.T) {
	home := t.TempDir()
	_ = Activate(home, Stanza{RequestID: "read-req", Name: "kube_read", GrantExpiresAt: "2099-01-01T00:00:00Z"})
	_ = Activate(home, Stanza{RequestID: "write-req", Name: "kube_write", GrantExpiresAt: "2099-01-02T00:00:00Z"})

	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube == nil || a.Kube.RequestID != "write-req" || a.Kube.Name != "kube_write" {
		t.Fatalf("kube stanza: %+v", a.Kube)
	}
	if a.AWS != nil {
		t.Fatalf("expected no aws stanza, got %+v", a.AWS)
	}
	if _, err := filepath.Abs(ActivePath(home)); err != nil {
		t.Fatal(err)
	}
}

func TestActivate_WhenKubeAndAWS_ItShouldKeepBothScopes(t *testing.T) {
	home := t.TempDir()
	_ = Activate(home, Stanza{RequestID: "k1", Name: "kube_read"})
	_ = Activate(home, Stanza{RequestID: "a1", Name: "aws_read"})

	a, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kube == nil || a.AWS == nil {
		t.Fatalf("expected both scopes: %+v", a)
	}
}
