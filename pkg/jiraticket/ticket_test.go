package jiraticket

import "testing"

func TestNormalize_WhenValidTicket_ItShouldReturnTicket(t *testing.T) {
	got, err := Normalize("ROSAENG-1234")
	if err != nil || got != "ROSAENG-1234" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestNormalize_WhenEmpty_ItShouldReturnEmpty(t *testing.T) {
	got, err := Normalize("  ")
	if err != nil || got != "" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestRequire_WhenMissing_ItShouldError(t *testing.T) {
	_, err := Require("")
	if err == nil {
		t.Fatal("expected error")
	}
}
