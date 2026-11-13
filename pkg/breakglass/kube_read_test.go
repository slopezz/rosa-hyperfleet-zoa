package breakglass

import (
	"slices"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

func TestKubeReadScope_WhenRegistered_ItShouldIncludeModernNetworkingAndStorage(t *testing.T) {
	def, ok := Get("kube-read")
	if !ok {
		t.Fatal("kube-read not registered")
	}
	want := map[string]string{
		"endpointslices":           "discovery.k8s.io",
		"persistentvolumes":        "",
		"ingresses":                "networking.k8s.io",
		"horizontalpodautoscalers": "autoscaling",
	}
	rules := def.RBAC(actions.TargetTypeMC)
	found := make(map[string]bool)
	for _, rule := range rules {
		ag := ""
		if len(rule.APIGroups) > 0 {
			ag = rule.APIGroups[0]
		}
		for _, res := range rule.Resources {
			if exp, ok := want[res]; ok && exp == ag {
				found[res] = true
			}
		}
	}
	for res := range want {
		if !found[res] {
			t.Fatalf("kube-read missing %q", res)
		}
	}
}

func TestKubeReadScope_WhenRegistered_ItShouldAllowServiceAccountRead(t *testing.T) {
	def, ok := Get("kube-read")
	if !ok {
		t.Fatal("kube-read not registered")
	}
	rules := def.RBAC(actions.TargetTypeRC)
	var hasSA bool
	for _, rule := range rules {
		if slices.Contains(rule.Resources, "serviceaccounts") {
			hasSA = true
		}
	}
	if !hasSA {
		t.Fatal("kube-read must include serviceaccounts get/list/watch")
	}
}

func TestKubeReadScope_WhenRegistered_ItShouldIncludePlatformOperatorAPIGroups(t *testing.T) {
	def, ok := Get("kube-read")
	if !ok {
		t.Fatal("kube-read not registered")
	}
	cases := []struct {
		target string
		group  string
	}{
		{actions.TargetTypeRC, "cert-manager.io"},
		{actions.TargetTypeRC, "external-secrets.io"},
		{actions.TargetTypeRC, "monitoring.thanos.io"},
		{actions.TargetTypeRC, "vpcresources.k8s.aws"},
		{actions.TargetTypeRC, "generators.external-secrets.io"},
		{actions.TargetTypeMC, "monitoring.coreos.com"},
		{actions.TargetTypeMC, "cert-manager.io"},
		{actions.TargetTypeMC, "external-secrets.io"},
		{actions.TargetTypeMC, "secrets-store.csi.x-k8s.io"},
		{actions.TargetTypeMC, "cluster.x-k8s.io"},
		{actions.TargetTypeMC, "infrastructure.cluster.x-k8s.io"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.target+"/"+tc.group, func(t *testing.T) {
			if !kubeReadHasAPIGroup(def.RBAC(tc.target), tc.group) {
				t.Fatalf("kube-read on %s missing platform API group %q", tc.target, tc.group)
			}
		})
	}
}

func kubeReadHasAPIGroup(rules []actions.RBACRule, group string) bool {
	for _, rule := range rules {
		if slices.Contains(rule.APIGroups, group) {
			return true
		}
	}
	return false
}

func TestKubeReadScope_WhenRegistered_ItShouldNotAllowWriteVerbs(t *testing.T) {
	def, ok := Get("kube-read")
	if !ok {
		t.Fatal("kube-read not registered")
	}
	for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
		for _, rule := range def.RBAC(target) {
			for _, verb := range rule.Verbs {
				if writeVerbs[verb] {
					t.Fatalf("read scope has write verb %q on target %s", verb, target)
				}
			}
		}
	}
	if err := ValidateScope(def); err != nil {
		t.Fatal(err)
	}
}
