package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/labels"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

func TestGarbageCollect_WhenTerminalExecutionExists_ItShouldCleanAndMark(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "exec-gc-1",
				Action:        "get_pods",
				Status:        store.StatusSucceeded,
				TargetCluster: "test-cluster",
			},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.garbageCollect(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.cleaned) != 1 {
		t.Fatalf("expected 1 cleaned, got %d", len(execStore.cleaned))
	}
	if execStore.cleaned[0] != "exec-gc-1" {
		t.Errorf("expected exec-gc-1, got %q", execStore.cleaned[0])
	}
}

func TestGarbageCollect_WhenNoTerminalExecutions_ItShouldDoNothing(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "exec-running",
				Action:        "get_pods",
				Status:        store.StatusDispatched,
				TargetCluster: "test-cluster",
			},
		},
	}

	cfg := testConfig()
	kubeForExec := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	exec := executor.New(kubeForExec, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	kubeForReconciler := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeForReconciler, nil, exec, cfg, noopLogger())

	err := r.garbageCollect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.cleaned) != 0 {
		t.Errorf("expected 0 cleaned, got %d", len(execStore.cleaned))
	}
}

func TestOrphanGC_WhenJobHasNoMatchingExecution_ItShouldCleanup(t *testing.T) {
	execStore := &mockExecutionStore{
		getResponses: map[string]*store.Execution{},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	// Create an old orphan job (>30 minutes old)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-orphan-123",
			Namespace: cfg.JobsNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "zoa",
				"zoa.openshift.io/execution-id": "orphan-123",
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.orphanGC(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cleanup attempt was made (job should be deleted)
	jobs, _ := kubeClient.BatchV1().Jobs(cfg.JobsNamespace).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("expected orphan job to be deleted, still have %d jobs", len(jobs.Items))
	}
}

func TestOrphanGC_WhenJobHasMatchingExecution_ItShouldSkip(t *testing.T) {
	execStore := &mockExecutionStore{
		getResponses: map[string]*store.Execution{
			"valid-123": {ID: "valid-123", Status: store.StatusDispatched},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-valid-123",
			Namespace: cfg.JobsNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "zoa",
				"zoa.openshift.io/execution-id": "valid-123",
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.orphanGC(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Job should NOT be deleted
	jobs, _ := kubeClient.BatchV1().Jobs(cfg.JobsNamespace).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Errorf("expected job to remain, got %d jobs", len(jobs.Items))
	}
}

func TestOrphanGC_WhenJobIsRecent_ItShouldSkip(t *testing.T) {
	execStore := &mockExecutionStore{
		getResponses: map[string]*store.Execution{},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	// Recent job (< 30 minutes old) — should NOT be GC'd even without a record
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-recent-456",
			Namespace: cfg.JobsNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "zoa",
				"zoa.openshift.io/execution-id": "recent-456",
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.orphanGC(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jobs, _ := kubeClient.BatchV1().Jobs(cfg.JobsNamespace).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Errorf("expected recent job to remain, got %d jobs", len(jobs.Items))
	}
}

func TestRunGC_WhenCalled_ItShouldExecuteGCPhases(t *testing.T) {
	execStore := &mockExecutionStore{
		executions:   []*store.Execution{},
		getResponses: map[string]*store.Execution{},
	}
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.RunGC(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGC_WhenTerminalQueryFails_ItShouldReturnError(t *testing.T) {
	execStore := &mockExecutionStore{
		terminalErr:  fmt.Errorf("dynamodb timeout"),
		getResponses: map[string]*store.Execution{},
	}
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.RunGC(ctx)
	if err == nil {
		t.Fatal("expected error when GC phase fails")
	}
}

func TestCleanupStaleMustGatherPods_WhenPodIsOld_ItShouldDelete(t *testing.T) {
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "must-gather-stale",
			Namespace: cfg.JobsNamespace,
			Labels: map[string]string{
				labels.KeyManagedBy:   labels.ValueManagedByZOA,
				labels.KeyComponent:   labels.ComponentMustGather,
				labels.KeyExecutionID: "stale-mg-1",
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
		},
		Status: corev1.PodStatus{Phase: corev1.PodFailed},
	}
	_, _ = kubeClient.CoreV1().Pods(cfg.JobsNamespace).Create(ctx, pod, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(&mockExecutionStore{}, kubeClient, nil, exec, cfg, noopLogger())

	if err := r.cleanupStaleMustGatherPods(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pods, err := kubeClient.CoreV1().Pods(cfg.JobsNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("expected stale must-gather pod to be deleted, got %d pods", len(pods.Items))
	}
}
