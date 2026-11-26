package scheduler

import (
	"context"
	"log/slog"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/labels"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

type mockBreakglassStore struct {
	items       []*store.BreakglassRequest
	getByID     map[string]*store.BreakglassRequest
	getErr      error
	markedClean []string
}

func (m *mockBreakglassStore) Create(_ context.Context, _ *store.BreakglassRequest) error { return nil }
func (m *mockBreakglassStore) Get(_ context.Context, requestID string) (*store.BreakglassRequest, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.getByID != nil {
		return m.getByID[requestID], nil
	}
	for _, item := range m.items {
		if item.RequestID == requestID {
			return item, nil
		}
	}
	return nil, nil
}
func (m *mockBreakglassStore) ListByOperator(_ context.Context, _ string, _ *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassStore) ListAll(_ context.Context, _ *store.BreakglassFilter) ([]*store.BreakglassRequest, error) {
	return nil, nil
}
func (m *mockBreakglassStore) MarkCleaned(_ context.Context, requestID string) error {
	m.markedClean = append(m.markedClean, requestID)
	return nil
}

func (m *mockBreakglassStore) AppendActivateAt(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockBreakglassStore) QueryByTargetAndStatus(_ context.Context, target string, status store.Status) ([]*store.BreakglassRequest, error) {
	var out []*store.BreakglassRequest
	for _, item := range m.items {
		if item.TargetCluster == target && item.Status == status {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *mockBreakglassStore) Transition(_ context.Context, requestID string, from, to store.Status, updates map[string]interface{}) error {
	for _, item := range m.items {
		if item.RequestID == requestID && item.Status == from {
			item.Status = to
			for k, v := range updates {
				if k == "expiredAt" {
					item.ExpiredAt = v.(string)
				}
			}
			return nil
		}
	}
	return nil
}

func newReconcilerWithBreakglass(t *testing.T, mock store.BreakglassStore, exec *executor.Executor, targetCluster string) *Reconciler {
	t.Helper()
	cfg := testConfig()
	cfg.TargetCluster = targetCluster
	r := NewReconciler(&mockExecutionStore{}, nil, nil, exec, cfg, slog.Default())
	r.SetBreakglassStore(mock)
	return r
}

func TestBreakglassExpireGrants_WhenPendingExpired_ItShouldMarkExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	mock := &mockBreakglassStore{
		items: []*store.BreakglassRequest{
			{
				RequestID:        "req-1",
				TargetCluster:    "mc01",
				Status:           store.StatusPending,
				RequestExpiresAt: past,
			},
		},
	}
	r := newReconcilerWithBreakglass(t, mock, nil, "mc01")
	if err := r.breakglassExpireGrants(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.items[0].Status != store.StatusExpired {
		t.Fatalf("expected expired, got %s", mock.items[0].Status)
	}
}

func TestBreakglassExpireGrants_WhenActiveGrantExpired_ItShouldExpireWithoutChangingApproval(t *testing.T) {
	past := time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	mock := &mockBreakglassStore{
		items: []*store.BreakglassRequest{
			{
				RequestID:      "req-2",
				TargetCluster:  "mc01",
				Status:         store.StatusActive,
				GrantExpiresAt: past,
				Approval:       store.WorkflowApprovalApproved,
				DecidedBy:      "bob",
			},
		},
	}
	r := newReconcilerWithBreakglass(t, mock, nil, "mc01")
	if err := r.breakglassExpireGrants(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.items[0].Status != store.StatusExpired {
		t.Fatalf("expected expired status, got %s", mock.items[0].Status)
	}
	if mock.items[0].Approval != store.WorkflowApprovalApproved {
		t.Fatalf("expected approval frozen, got %s", mock.items[0].Approval)
	}
	if mock.items[0].DecidedBy != "bob" {
		t.Fatalf("expected decided_by unchanged, got %q", mock.items[0].DecidedBy)
	}
	if mock.items[0].ExpiredAt == "" {
		t.Fatal("expected expired_at to be set")
	}
}

func TestBreakglassOrphanGC_WhenSANoDynamoRow_ItShouldCleanup(t *testing.T) {
	ctx := context.Background()
	const ns = "zoa-jobs"
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // fake clientset without apply
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-bg-orphan-abc",
			Namespace: ns,
			Labels: map[string]string{
				labels.KeyManagedBy: labels.ValueManagedByZOA,
				labels.KeyRequestID: "orphan-abc",
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
	}
	_, _ = kubeClient.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, slog.Default())
	mock := &mockBreakglassStore{getByID: map[string]*store.BreakglassRequest{}}
	r := newReconcilerWithBreakglass(t, mock, exec, "mc01")
	if err := r.breakglassOrphanGC(ctx); err != nil {
		t.Fatalf("orphanGC: %v", err)
	}
	_, err := kubeClient.CoreV1().ServiceAccounts(ns).Get(ctx, "zoa-bg-orphan-abc", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected orphan service account to be deleted")
	}
}

func TestBreakglassOrphanGC_WhenGrantStillActive_ItShouldNotDelete(t *testing.T) {
	ctx := context.Background()
	const ns = "zoa-jobs"
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-bg-active-xyz",
			Namespace: ns,
			Labels: map[string]string{
				labels.KeyManagedBy: labels.ValueManagedByZOA,
				labels.KeyRequestID: "active-xyz",
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
	}
	_, _ = kubeClient.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, slog.Default())
	mock := &mockBreakglassStore{
		getByID: map[string]*store.BreakglassRequest{
			"active-xyz": {
				RequestID:     "active-xyz",
				TargetCluster: "mc01",
				Status:        store.StatusActive,
			},
		},
	}
	r := newReconcilerWithBreakglass(t, mock, exec, "mc01")
	if err := r.breakglassOrphanGC(ctx); err != nil {
		t.Fatalf("orphanGC: %v", err)
	}
	if _, err := kubeClient.CoreV1().ServiceAccounts(ns).Get(ctx, "zoa-bg-active-xyz", metav1.GetOptions{}); err != nil {
		t.Fatalf("expected active grant SA to remain: %v", err)
	}
}