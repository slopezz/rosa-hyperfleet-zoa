package cli

import (
	"strings"
	"testing"
)

func TestMergeAWSCredentialsProfile_WhenReplacing_ItShouldOverwriteSection(t *testing.T) {
	existing := "[zoa-bg-old]\naws_access_key_id = OLD\n\n[zoa-bg-other]\naws_access_key_id = X\n"
	block := "[zoa-bg-old]\naws_access_key_id = NEW\n"
	got := mergeAWSCredentialsProfile(existing, "zoa-bg-old", block)
	if !strings.Contains(got, "aws_access_key_id = NEW") {
		t.Fatalf("missing NEW: %s", got)
	}
	if strings.Contains(got, "aws_access_key_id = OLD") {
		t.Fatalf("still has OLD: %s", got)
	}
	if !strings.Contains(got, "zoa-bg-other") {
		t.Fatalf("dropped other profile: %s", got)
	}
}
