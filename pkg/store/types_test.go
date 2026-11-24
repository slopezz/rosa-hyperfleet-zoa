package store

import (
	"testing"
	"time"
)

func TestStatus_WhenTerminal_ItShouldReturnTrue(t *testing.T) {
	terminals := []Status{StatusSucceeded, StatusFailed, StatusTimedOut, StatusRejected, StatusExpired, StatusDenied}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Errorf("expected %q to be terminal", s)
		}
	}
}

func TestStatus_WhenNonTerminal_ItShouldReturnFalse(t *testing.T) {
	nonTerminals := []Status{StatusPendingApproval, StatusApproved, StatusDispatched, StatusPending, StatusActive}
	for _, s := range nonTerminals {
		if s.IsTerminal() {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}

func TestCreatedAtTime_WhenValidRFC3339_ItShouldParseSuccessfully(t *testing.T) {
	exec := &Execution{CreatedAt: "2024-06-15T10:30:00.123456789Z"}
	ts, err := exec.CreatedAtTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.Year() != 2024 || ts.Month() != time.June || ts.Day() != 15 {
		t.Errorf("unexpected date: %v", ts)
	}
}

func TestCreatedAtTime_WhenEmpty_ItShouldReturnError(t *testing.T) {
	exec := &Execution{CreatedAt: ""}
	_, err := exec.CreatedAtTime()
	if err == nil {
		t.Fatal("expected error for empty CreatedAt")
	}
}

func TestCreatedAtTime_WhenInvalidFormat_ItShouldReturnError(t *testing.T) {
	exec := &Execution{CreatedAt: "not-a-timestamp"}
	_, err := exec.CreatedAtTime()
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestDispatchedAtTime_WhenValid_ItShouldParseSuccessfully(t *testing.T) {
	exec := &Execution{DispatchedAt: "2024-06-15T10:30:05.000Z"}
	ts, err := exec.DispatchedAtTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.Second() != 5 {
		t.Errorf("expected second=5, got %d", ts.Second())
	}
}

func TestDispatchedAtTime_WhenEmpty_ItShouldReturnError(t *testing.T) {
	exec := &Execution{DispatchedAt: ""}
	_, err := exec.DispatchedAtTime()
	if err == nil {
		t.Fatal("expected error for empty DispatchedAt")
	}
}
