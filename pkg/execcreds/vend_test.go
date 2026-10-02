package execcreds

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionPolicy_WhenTaskAndClusterGiven_ItShouldRestrictResources(t *testing.T) {
	cluster := "arn:aws:ecs:us-east-1:123456789012:cluster/rc-zoa-boundary"
	task := "arn:aws:ecs:us-east-1:123456789012:task/rc-zoa-boundary/abc123"

	policy, err := sessionPolicy(cluster, task)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(policy), &doc); err != nil {
		t.Fatal(err)
	}
	stmt, ok := doc["Statement"].([]interface{})
	if !ok || len(stmt) < 3 {
		t.Fatalf("expected at least 3 statements, got %v", doc["Statement"])
	}
	if !strings.Contains(policy, task) || !strings.Contains(policy, cluster) {
		t.Errorf("policy should include task and cluster ARNs: %s", policy)
	}
}

func TestSanitizeSessionName_WhenInvalidChars_ItShouldReplace(t *testing.T) {
	got := sanitizeSessionName("slopezma@example.com")
	if got == "" {
		t.Fatal("expected non-empty session name")
	}
	if len(got) > 64 {
		t.Fatalf("session name too long: %d", len(got))
	}
}

func TestVendForTask_WhenVendorNil_ItShouldError(t *testing.T) {
	var v *Vendor
	if _, err := v.VendForTask(t.Context(), "arn:aws:iam::123:role/test-exec-scoped", "user", "cluster", "task"); err == nil {
		t.Fatal("expected error")
	}
}
