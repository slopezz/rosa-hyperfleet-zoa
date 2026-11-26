package profilecatalog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestFormatDescribeHuman_WhenProfileSet_ItShouldIncludeRequestSection(t *testing.T) {
	var buf bytes.Buffer
	FormatDescribeHuman(&buf, &client.ProfileMetadata{
		Name:        "kube_read",
		Scope:       "kube",
		Type:        "read",
		Description: "read only",
		Examples:    []string{"zoa breakglass request kube_read --reason ROSAENG-1234"},
	})
	out := buf.String()
	for _, sub := range []string{
		"REQUEST (global flags on every zoa breakglass request):",
		"--reason",
		"--wait",
		"EXAMPLES:",
	} {
		if !strings.Contains(out, sub) {
			t.Fatalf("expected describe to contain %q\n%s", sub, out)
		}
	}
}