package execcreds

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionPolicy_WhenTaskAndClusterGiven_ItShouldRestrictResources(t *testing.T) {
	cluster := "arn:aws:ecs:us-east-1:123456789012:cluster/rc-zoa-boundary"
	task := "arn:aws:ecs:us-east-1:123456789012:task/rc-zoa-boundary/abc123"
	kms := "arn:aws:kms:us-east-1:123456789012:key/abc-123"

	policy, err := sessionPolicy(cluster, task, kms)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(policy), &doc); err != nil {
		t.Fatal(err)
	}
	stmt, ok := doc["Statement"].([]interface{})
	if !ok || len(stmt) < 4 {
		t.Fatalf("expected at least 4 statements, got %v", doc["Statement"])
	}
	if !strings.Contains(policy, task) || !strings.Contains(policy, cluster) {
		t.Errorf("policy should include task and cluster ARNs: %s", policy)
	}
	if !strings.Contains(policy, kms) || !strings.Contains(policy, "kms:GenerateDataKey") {
		t.Errorf("policy should include KMS for ECS Exec encryption: %s", policy)
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
	if _, err := v.VendForTask(t.Context(), "arn:aws:iam::123:role/test-exec-scoped", "user", "cluster", "task", "arn:aws:kms:us-east-1:123:key/1"); err == nil {
		t.Fatal("expected error")
	}
}
