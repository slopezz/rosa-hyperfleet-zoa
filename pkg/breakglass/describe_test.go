package breakglass

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestDescribeScope_WhenKubeWrite_ItShouldIncludeEffectiveRBAC(t *testing.T) {
	def, ok := Get("kube-write")
	if !ok {
		t.Fatal("kube-write not registered")
	}
	desc := DescribeScope(def, actions.TargetTypeMC)
	if len(desc.RBAC) == 0 {
		t.Fatal("expected rbac rules on describe")
	}
	if len(desc.Composition) == 0 {
		t.Fatal("expected composition lines")
	}
}

func TestDescribeScope_WhenAwsRead_ItShouldIncludeIAMComposition(t *testing.T) {
	def, ok := Get("aws-read")
	if !ok {
		t.Fatal("aws-read not registered")
	}
	desc := DescribeScope(def, "")
	if len(desc.IAMComposition) == 0 {
		t.Fatal("expected iam_composition")
	}
	if len(desc.RBAC) != 0 {
		t.Fatal("aws scope must not expose kube rbac")
	}
}
