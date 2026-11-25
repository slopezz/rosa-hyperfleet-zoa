package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

func TestWriteBreakglassGrantTooShort_WhenRemainingBelowKubeMin_ItShouldReturn409(t *testing.T) {
	w := httptest.NewRecorder()
	err := &breakglass.GrantTooShortError{
		Plane:         breakglass.PlaneKube,
		Remaining:     2 * time.Minute,
		MinCredential: 10 * time.Minute,
	}
	if !writeBreakglassGrantTooShort(w, err) {
		t.Fatal("expected grant too short handler")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "grant_too_short" {
		t.Fatalf("code = %q", body["code"])
	}
	if body["reason"] == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestWriteBreakglassGrantTooShort_WhenUnrelatedError_ItShouldReturnFalse(t *testing.T) {
	w := httptest.NewRecorder()
	if writeBreakglassGrantTooShort(w, breakglass.ErrGrantTooShortForCredential) {
		t.Fatal("expected false without GrantTooShortError")
	}
}
