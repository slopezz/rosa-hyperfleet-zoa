package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBreakglassAWSProfile_WhenCalled_ItShouldWriteDefaultProfileOnly(t *testing.T) {
	home := t.TempDir()
	aws := map[string]interface{}{
		"access_key_id":     "AKIATEST",
		"secret_access_key": "secret",
		"session_token":     "token",
		"region":            "us-east-1",
	}
	if err := writeBreakglassAWSProfile(home, "req-1", aws); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "zoa", "breakglass", "aws-credentials"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "[default]") {
		t.Fatalf("expected [default]: %s", body)
	}
	if strings.Contains(body, "zoa-bg-") {
		t.Fatalf("expected no named bg profile: %s", body)
	}
	if !strings.Contains(body, "AKIATEST") {
		t.Fatalf("missing access key: %s", body)
	}
	if err := writeBreakglassAWSProfile(home, "req-2", map[string]interface{}{
		"access_key_id":     "AKIASECOND",
		"secret_access_key": "s2",
		"session_token":     "t2",
	}); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(filepath.Join(home, ".config", "zoa", "breakglass", "aws-credentials"))
	if strings.Contains(string(data2), "AKIATEST") {
		t.Fatalf("expected replace not merge: %s", data2)
	}
}
