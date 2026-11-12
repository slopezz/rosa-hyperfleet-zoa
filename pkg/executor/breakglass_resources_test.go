package executor

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/labels"
)

func TestEnsureBreakglassKube_WhenClusterScoped_ItShouldUseZoaBgNaming(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // fake clientset without apply configs
	e := &Executor{kubeClient: client, logger: noopLogger(), targetType: "rc"}
	ctx := context.Background()

	_, _ = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}, metav1.CreateOptions{})

	saName, err := e.EnsureBreakglassKube(ctx, "req-1", "kube-read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "zoa-bg-req-1"
	if saName != want {
		t.Fatalf("expected sa %q, got %q", want, saName)
	}

	sa, err := client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, want, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("sa not found: %v", err)
	}
	if sa.Labels[labels.KeyRequestID] != "req-1" {
		t.Fatalf("expected request-id label, got %v", sa.Labels)
	}

	cr, err := client.RbacV1().ClusterRoles().Get(ctx, want, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("cluster role not found: %v", err)
	}
	if cr.Labels[labels.KeyRequestID] != "req-1" {
		t.Fatalf("expected request-id on cluster role")
	}

	crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, want, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("crb not found: %v", err)
	}
	if crb.Subjects[0].Name != want {
		t.Fatalf("expected subject %q", want)
	}
}

func TestCleanupBreakglass_WhenResourcesExist_ItShouldRemoveThem(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck
	e := &Executor{kubeClient: client, logger: noopLogger(), targetType: "rc"}
	ctx := context.Background()

	_, _ = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}, metav1.CreateOptions{})

	rbac := &actions.RBACConfig{
		ClusterScoped: true,
		Rules:         []actions.RBACRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}},
	}
	if err := e.createBreakglassResourcesIdempotent(ctx, "req-2", breakglassResourceName("req-2"), rbac); err != nil {
		t.Fatalf("setup: %v", err)
	}

	e.CleanupBreakglass(ctx, "req-2")

	_, err := client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, "zoa-bg-req-2", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected service account deleted")
	}
	_, err = client.RbacV1().ClusterRoles().Get(ctx, "zoa-bg-req-2", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected cluster role deleted")
	}
}
