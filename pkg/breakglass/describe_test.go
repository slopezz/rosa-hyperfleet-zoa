package breakglass

import (
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestDescribeProfile_WhenKubeWrite_ItShouldIncludeEffectiveRBAC(t *testing.T) {
	def, ok := Get("kube_write")
	if !ok {
		t.Fatal("kube_write not registered")
	}
	desc := DescribeProfile(def, actions.TargetTypeMC)
	if len(desc.RBAC) == 0 {
		t.Fatal("expected rbac rules on describe")
	}
	if len(desc.Capabilities) == 0 {
		t.Fatal("expected capabilities")
	}
}

func TestDescribeProfile_WhenAwsRead_ItShouldIncludeIAMComposition(t *testing.T) {
	def, ok := Get("aws_read")
	if !ok {
		t.Fatal("aws_read not registered")
	}
	desc := DescribeProfile(def, "")
	if len(desc.IAMComposition) == 0 {
		t.Fatal("expected iam_composition")
	}
	if len(desc.RBAC) != 0 {
		t.Fatal("aws scope must not expose kube rbac")
	}
}
