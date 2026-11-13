package breakglass

import (
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestDescribeScope_WhenAwsWriteOnMc_ItShouldNoteEquivalentToRead(t *testing.T) {
	def, ok := Get("aws-write")
	if !ok {
		t.Fatal("aws-write not registered")
	}
	desc := DescribeScope(def, actions.TargetTypeMC)
	found := false
	for _, line := range desc.IAMComposition {
		if strings.Contains(line, "MC: ReadOnlyAccess only") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected MC note in IAM composition, got %v", desc.IAMComposition)
	}
}

func TestDescribeScope_WhenAwsReadOnRc_ItShouldNotAppendTerraformPath(t *testing.T) {
	def, ok := Get("aws-read")
	if !ok {
		t.Fatal("aws-read not registered")
	}
	desc := DescribeScope(def, actions.TargetTypeRC)
	for _, line := range desc.IAMComposition {
		if strings.Contains(line, "breakglass_iam.tf") || strings.HasPrefix(line, "IAM:") {
			t.Fatalf("unexpected IAM catalog line on RC aws-read: %q", line)
		}
	}
}
