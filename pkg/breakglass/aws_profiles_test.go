package breakglass

import (
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestAwsWriteProfile_WhenRegistered_ItShouldDocumentMcLimitInRestrictions(t *testing.T) {
	def, ok := Get("aws_write")
	if !ok {
		t.Fatal("aws_write not registered")
	}
	found := false
	for _, line := range def.Metadata.Restrictions {
		if strings.Contains(line, "MC target:") && strings.Contains(line, "aws_read") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MC note in restrictions, got %v", def.Metadata.Restrictions)
	}
	if len(def.Metadata.IAMComposition) != 1 || !strings.Contains(def.Metadata.IAMComposition[0], "Role:") {
		t.Fatalf("expected single role IAM line, got %v", def.Metadata.IAMComposition)
	}
}

func TestDescribeProfile_WhenAwsReadOnRc_ItShouldNotAppendTerraformPath(t *testing.T) {
	def, ok := Get("aws_read")
	if !ok {
		t.Fatal("aws_read not registered")
	}
	desc := DescribeProfile(def, actions.TargetTypeRC)
	for _, line := range desc.IAMComposition {
		if strings.Contains(line, "breakglass_iam.tf") || strings.HasPrefix(line, "IAM:") {
			t.Fatalf("unexpected IAM catalog line on RC aws_read: %q", line)
		}
	}
}
