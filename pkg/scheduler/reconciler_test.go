package scheduler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/config"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/executor"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/store"
)

// --- Mocks ---

type mockExecutionStore struct {
	executions   []*store.Execution
	transitions  []transitionRecord
	cleaned      []string
	getResponses map[string]*store.Execution
	queryErr     error
	terminalErr  error
}

type transitionRecord struct {
	ID       string
	From     store.Status
	To       store.Status
	Metadata map[string]interface{}
}

func (m *mockExecutionStore) Create(_ context.Context, _ *store.Execution) error { return nil }

func (m *mockExecutionStore) Get(_ context.Context, id string) (*store.Execution, error) {
	if m.getResponses != nil {
		return m.getResponses[id], nil
	}
	for _, e := range m.executions {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (m *mockExecutionStore) List(_ context.Context, _ string, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return m.executions, nil
}
func (m *mockExecutionStore) ListAll(_ context.Context, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return m.executions, nil
}

func (m *mockExecutionStore) TransitionStatus(_ context.Context, id string, from, to store.Status) error {
	m.transitions = append(m.transitions, transitionRecord{ID: id, From: from, To: to})
	return nil
}

func (m *mockExecutionStore) TransitionWithMetadata(_ context.Context, id string, from, to store.Status, metadata map[string]interface{}) error {
	m.transitions = append(m.transitions, transitionRecord{ID: id, From: from, To: to, Metadata: metadata})
	return nil
}

func (m *mockExecutionStore) QueryByStatus(_ context.Context, status store.Status) ([]*store.Execution, error) {
	var result []*store.Execution
	for _, e := range m.executions {
		if e.Status == status {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockExecutionStore) QueryByStatusAndClass(_ context.Context, status store.Status, class string) ([]*store.Execution, error) {
	var result []*store.Execution
	for _, e := range m.executions {
		if e.Status == status && e.ExecutionMode == class {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockExecutionStore) QueryTerminal(_ context.Context, _ time.Duration) ([]*store.Execution, error) {
	var result []*store.Execution
	for _, e := range m.executions {
		if e.Status.IsTerminal() && !e.Cleaned {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockExecutionStore) MarkCleaned(_ context.Context, id string) error {
	m.cleaned = append(m.cleaned, id)
	return nil
}

func (m *mockExecutionStore) ListByTargetAndAction(_ context.Context, _, _ string, _ time.Time) ([]*store.Execution, error) {
	return nil, nil
}

func (m *mockExecutionStore) CountActiveByTarget(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockExecutionStore) QueryByTargetAndStatus(_ context.Context, target string, status store.Status) ([]*store.Execution, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	var result []*store.Execution
	for _, e := range m.executions {
		if e.TargetCluster == target && e.Status == status {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockExecutionStore) QueryTerminalByTarget(_ context.Context, target string, _ time.Duration) ([]*store.Execution, error) {
	if m.terminalErr != nil {
		return nil, m.terminalErr
	}
	var result []*store.Execution
	for _, e := range m.executions {
		if e.TargetCluster == target && e.Status.IsTerminal() && !e.Cleaned {
			result = append(result, e)
		}
	}
	return result, nil
}

type mockLambdaClient struct {
	invocations []lambda.InvokeInput
	err         error
}

func (m *mockLambdaClient) Invoke(_ context.Context, params *lambda.InvokeInput, _ ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
	m.invocations = append(m.invocations, *params)
	return &lambda.InvokeOutput{StatusCode: 202}, m.err
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() *config.Config {
	return &config.Config{
		HandlerMode:                    "worker",
		TargetCluster:                  "test-cluster",
		TargetType:                     "mc",
		WorkerFunctionName:             "zoa-test-worker",
		MaxBatchPerTick:                30,
		ReconcilerDeadlineSeconds:      55,
		ExecutionDeadlineSeconds:       295,
		JobsNamespace:                  "zoa-jobs",
		AsyncSchedulingOverheadSeconds: 180,
	}
}

// --- Tests ---

func TestDispatchApproved_WhenApprovedExecutionsExist_ItShouldSelfInvokeWorkerLambda(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{ID: "exec-001", Action: "delete-pod", Status: store.StatusApproved, TargetCluster: "test-cluster"},
			{ID: "exec-002", Action: "get-resource", Status: store.StatusApproved, TargetCluster: "test-cluster"},
		},
	}
	lambdaClient := &mockLambdaClient{}
	cfg := testConfig()

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, lambdaClient, nil, cfg, noopLogger())

	err := r.dispatchApproved(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lambdaClient.invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(lambdaClient.invocations))
	}
	if *lambdaClient.invocations[0].FunctionName != "zoa-test-worker" {
		t.Errorf("wrong function name: %s", *lambdaClient.invocations[0].FunctionName)
	}

	// Verify transitions: approved → dispatched
	if len(execStore.transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(execStore.transitions))
	}
	for _, tr := range execStore.transitions {
		if tr.From != store.StatusApproved || tr.To != store.StatusDispatched {
			t.Errorf("wrong transition: %s → %s", tr.From, tr.To)
		}
	}
}

func TestDispatchApproved_WhenNoApprovedExecutions_ItShouldDoNothing(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{ID: "exec-001", Status: store.StatusDispatched, TargetCluster: "test-cluster"},
		},
	}
	lambdaClient := &mockLambdaClient{}
	cfg := testConfig()

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, lambdaClient, nil, cfg, noopLogger())

	err := r.dispatchApproved(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lambdaClient.invocations) != 0 {
		t.Errorf("expected 0 invocations, got %d", len(lambdaClient.invocations))
	}
}

func TestDispatchApproved_WhenBatchLimitReached_ItShouldStopProcessing(t *testing.T) {
	executions := make([]*store.Execution, 50)
	for i := range 50 {
		executions[i] = &store.Execution{
			ID:            fmt.Sprintf("exec-%03d", i),
			Action:        "get-resource",
			Status:        store.StatusApproved,
			TargetCluster: "test-cluster",
		}
	}

	execStore := &mockExecutionStore{executions: executions}
	lambdaClient := &mockLambdaClient{}
	cfg := testConfig()
	cfg.MaxBatchPerTick = 5

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, lambdaClient, nil, cfg, noopLogger())

	err := r.dispatchApproved(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lambdaClient.invocations) != 5 {
		t.Errorf("expected 5 invocations (batch limit), got %d", len(lambdaClient.invocations))
	}
}

func TestDispatchApproved_WhenInvokeFails_ItShouldRollBackToApproved(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{ID: "exec-001", Action: "delete-pod", Status: store.StatusApproved, TargetCluster: "test-cluster"},
		},
	}
	lambdaClient := &mockLambdaClient{err: fmt.Errorf("lambda invoke error")}
	cfg := testConfig()

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, lambdaClient, nil, cfg, noopLogger())

	err := r.dispatchApproved(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 transitions: approved→dispatched (claim) then dispatched→approved (rollback)
	if len(execStore.transitions) != 2 {
		t.Fatalf("expected 2 transitions (claim + rollback), got %d", len(execStore.transitions))
	}
	if execStore.transitions[0].From != store.StatusApproved || execStore.transitions[0].To != store.StatusDispatched {
		t.Errorf("first transition should be approved→dispatched, got %s→%s", execStore.transitions[0].From, execStore.transitions[0].To)
	}
	if execStore.transitions[1].From != store.StatusDispatched || execStore.transitions[1].To != store.StatusApproved {
		t.Errorf("second transition should be dispatched→approved (rollback), got %s→%s", execStore.transitions[1].From, execStore.transitions[1].To)
	}
}

func TestDispatchApproved_WhenContextCancelled_ItShouldStopEarly(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{ID: "exec-001", Action: "delete-pod", Status: store.StatusApproved, TargetCluster: "test-cluster"},
			{ID: "exec-002", Action: "delete-pod", Status: store.StatusApproved, TargetCluster: "test-cluster"},
		},
	}
	lambdaClient := &mockLambdaClient{}
	cfg := testConfig()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancelled

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, lambdaClient, nil, cfg, noopLogger())

	err := r.dispatchApproved(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lambdaClient.invocations) != 0 {
		t.Errorf("expected 0 invocations when context cancelled, got %d", len(lambdaClient.invocations))
	}
}

func TestTimeoutDispatched_WhenExecutionExceedsTimeout_ItShouldMarkTimedOut(t *testing.T) {
	oldTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "exec-timeout",
				Action:         "get-resource",
				Status:         store.StatusDispatched,
				TimeoutSeconds: 60,
				DispatchedAt:   oldTime,
				TargetCluster:  "test-cluster",
			},
		},
	}
	cfg := testConfig()

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.timeoutDispatched(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(execStore.transitions))
	}
	if execStore.transitions[0].To != store.StatusTimedOut {
		t.Errorf("expected transition to timed_out, got %s", execStore.transitions[0].To)
	}
}

func TestTimeoutDispatched_WhenExecutionWithinTimeout_ItShouldNotTransition(t *testing.T) {
	recentTime := time.Now().Add(-10 * time.Second).Format(time.RFC3339Nano)
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "exec-recent",
				Action:         "get-resource",
				Status:         store.StatusDispatched,
				TimeoutSeconds: 120,
				DispatchedAt:   recentTime,
				TargetCluster:  "test-cluster",
			},
		},
	}
	cfg := testConfig()

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.timeoutDispatched(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 0 {
		t.Errorf("expected 0 transitions for non-expired execution, got %d", len(execStore.transitions))
	}
}

func TestPollAsyncJobs_WhenJobCompleted_ItShouldTransitionToSucceeded(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-ok",
				Action:        "get-resource",
				Status:        store.StatusDispatched,
				ExecutionMode: "async",
				DispatchedAt:  time.Now().Add(-30 * time.Second).Format(time.RFC3339Nano),
				TargetCluster: "test-cluster",
			},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-async-ok",
			Namespace: cfg.JobsNamespace,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: "True"},
			},
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.pollAsyncJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(execStore.transitions))
	}
	if execStore.transitions[0].To != store.StatusSucceeded {
		t.Errorf("expected transition to succeeded, got %s", execStore.transitions[0].To)
	}
}

func TestPollAsyncJobs_WhenJobFailed_ItShouldTransitionToFailed(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-fail",
				Action:        "delete-pod",
				Status:        store.StatusDispatched,
				ExecutionMode: "async",
				DispatchedAt:  time.Now().Add(-60 * time.Second).Format(time.RFC3339Nano),
				TargetCluster: "test-cluster",
			},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-async-fail",
			Namespace: cfg.JobsNamespace,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: "True"},
			},
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.pollAsyncJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(execStore.transitions))
	}
	if execStore.transitions[0].To != store.StatusFailed {
		t.Errorf("expected transition to failed, got %s", execStore.transitions[0].To)
	}
}

func TestPollAsyncJobs_WhenJobStillRunning_ItShouldNotTransition(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-running",
				Action:        "delete-pod",
				Status:        store.StatusDispatched,
				ExecutionMode: "async",
				DispatchedAt:  time.Now().Add(-5 * time.Second).Format(time.RFC3339Nano),
				TargetCluster: "test-cluster",
			},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	// Job exists but no terminal condition
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-async-running",
			Namespace: cfg.JobsNamespace,
		},
		Status: batchv1.JobStatus{
			Active: 1,
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.pollAsyncJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 0 {
		t.Errorf("expected 0 transitions for running job, got %d", len(execStore.transitions))
	}
}

func TestTimeoutDispatched_WhenAsyncWithinOverhead_ItShouldNotTimeout(t *testing.T) {
	// Async TA with 30s timeout dispatched 2 minutes ago.
	// Without overhead: 2m > 30s → timed_out. With 180s overhead: 2m < 30s+180s → safe.
	dispatchedAt := time.Now().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-ok",
				Action:         "get-resource",
				Status:         store.StatusDispatched,
				TimeoutSeconds: 30,
				ExecutionMode:  "async",
				DispatchedAt:   dispatchedAt,
				TargetCluster:  "test-cluster",
			},
		},
	}
	cfg := testConfig()
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.timeoutDispatched(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(execStore.transitions) != 0 {
		t.Errorf("expected 0 transitions (async within overhead), got %d", len(execStore.transitions))
	}
}

func TestTimeoutDispatched_WhenAsyncExceedsOverhead_ItShouldTimeout(t *testing.T) {
	// Async TA with 30s timeout dispatched 5 minutes ago.
	// 5m > 30s + 180s (3.5m total) → timed_out.
	dispatchedAt := time.Now().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-expired",
				Action:         "get-resource",
				Status:         store.StatusDispatched,
				TimeoutSeconds: 30,
				ExecutionMode:  "async",
				DispatchedAt:   dispatchedAt,
				TargetCluster:  "test-cluster",
			},
		},
	}
	cfg := testConfig()
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.timeoutDispatched(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition for expired async, got %d", len(execStore.transitions))
	}
	if execStore.transitions[0].To != store.StatusTimedOut {
		t.Errorf("expected transition to timed_out, got %s", execStore.transitions[0].To)
	}
}

func TestTimeoutDispatched_WhenSyncExceedsTimeout_ItShouldNotAddOverhead(t *testing.T) {
	// Sync TA with 30s timeout dispatched 45s ago.
	// 45s > 30s → timed_out. No overhead added for sync.
	dispatchedAt := time.Now().Add(-45 * time.Second).Format(time.RFC3339Nano)
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "sync-expired",
				Action:         "get-resource",
				Status:         store.StatusDispatched,
				TimeoutSeconds: 30,
				ExecutionMode:  "sync",
				DispatchedAt:   dispatchedAt,
				TargetCluster:  "test-cluster",
			},
		},
	}
	cfg := testConfig()
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	r := NewReconciler(execStore, kubeClient, nil, nil, cfg, noopLogger())

	err := r.timeoutDispatched(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition for expired sync, got %d", len(execStore.transitions))
	}
	if execStore.transitions[0].To != store.StatusTimedOut {
		t.Errorf("expected transition to timed_out, got %s", execStore.transitions[0].To)
	}
}

func TestJobStatus_WhenCompleteCondition_ItShouldReturnSucceeded(t *testing.T) {
	job := &batchv1.Job{
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: "True"},
			},
		},
	}
	if s := jobStatus(job); s != store.StatusSucceeded {
		t.Errorf("expected succeeded, got %s", s)
	}
}

func TestJobStatus_WhenFailedCondition_ItShouldReturnFailed(t *testing.T) {
	job := &batchv1.Job{
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: "True"},
			},
		},
	}
	if s := jobStatus(job); s != store.StatusFailed {
		t.Errorf("expected failed, got %s", s)
	}
}

func TestJobStatus_WhenNoConditions_ItShouldReturnEmpty(t *testing.T) {
	job := &batchv1.Job{
		Status: batchv1.JobStatus{Active: 1},
	}
	if s := jobStatus(job); s != "" {
		t.Errorf("expected empty status for active job, got %s", s)
	}
}

// --- S3 mock for ArtifactSizes integration ---

type mockS3ForReconciler struct {
	headResults map[string]*s3.HeadObjectOutput
}

func (m *mockS3ForReconciler) PutObject(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3ForReconciler) HeadObject(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if params.Key != nil {
		if result, ok := m.headResults[*params.Key]; ok {
			return result, nil
		}
	}
	return nil, fmt.Errorf("NoSuchKey")
}

func TestPollAsyncJobs_WhenJobCompleted_ItShouldEnrichWithArtifactSizes(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-with-artifacts",
				Action:        "get-resource",
				Status:        store.StatusDispatched,
				ExecutionMode: "async",
				DispatchedAt:  time.Now().Add(-60 * time.Second).Format(time.RFC3339Nano),
				TargetCluster: "test-cluster",
			},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-async-with-artifacts",
			Namespace: cfg.JobsNamespace,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: "True"},
			},
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	outputSize := int64(617)
	logSize := int64(780)
	s3Mock := &mockS3ForReconciler{
		headResults: map[string]*s3.HeadObjectOutput{
			"executions/async-with-artifacts/output.json":   {ContentLength: &outputSize},
			"executions/async-with-artifacts/execution.log": {ContentLength: &logSize},
		},
	}

	exec := executor.New(nil, nil, s3Mock, nil, executor.ExecutorConfig{
		ArtifactBucket: "test-bucket",
	}, noopLogger())

	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.pollAsyncJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(execStore.transitions))
	}
	tr := execStore.transitions[0]
	if tr.To != store.StatusSucceeded {
		t.Errorf("expected transition to succeeded, got %s", tr.To)
	}
	if tr.Metadata == nil {
		t.Fatal("expected metadata in transition")
	}
	if tr.Metadata["outputBytes"] != int64(617) {
		t.Errorf("expected outputBytes=617, got %v", tr.Metadata["outputBytes"])
	}
	if tr.Metadata["logBytes"] != int64(780) {
		t.Errorf("expected logBytes=780, got %v", tr.Metadata["logBytes"])
	}
	if tr.Metadata["outputFormat"] != "json" {
		t.Errorf("expected outputFormat='json', got %v", tr.Metadata["outputFormat"])
	}
}

func TestPollAsyncJobs_WhenJobCompletedWithTarGz_ItShouldDetectFormat(t *testing.T) {
	execStore := &mockExecutionStore{
		executions: []*store.Execution{
			{
				ID: "async-tar",
				Action:        "must-gather",
				Status:        store.StatusDispatched,
				ExecutionMode: "async",
				DispatchedAt:  time.Now().Add(-120 * time.Second).Format(time.RFC3339Nano),
				TargetCluster: "test-cluster",
			},
		},
	}

	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-async-tar",
			Namespace: cfg.JobsNamespace,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: "True"},
			},
		},
	}
	_, _ = kubeClient.BatchV1().Jobs(cfg.JobsNamespace).Create(ctx, job, metav1.CreateOptions{})

	tarSize := int64(5242880)
	s3Mock := &mockS3ForReconciler{
		headResults: map[string]*s3.HeadObjectOutput{
			"executions/async-tar/output.tar.gz": {ContentLength: &tarSize},
		},
	}

	exec := executor.New(nil, nil, s3Mock, nil, executor.ExecutorConfig{
		ArtifactBucket: "test-bucket",
	}, noopLogger())

	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.pollAsyncJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(execStore.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(execStore.transitions))
	}
	tr := execStore.transitions[0]
	if tr.Metadata["outputBytes"] != int64(5242880) {
		t.Errorf("expected outputBytes=5242880, got %v", tr.Metadata["outputBytes"])
	}
	if tr.Metadata["outputFormat"] != "tar.gz" {
		t.Errorf("expected outputFormat='tar.gz', got %v", tr.Metadata["outputFormat"])
	}
}

func TestRun_WhenDispatchPhaseFails_ItShouldReturnError(t *testing.T) {
	execStore := &mockExecutionStore{
		queryErr:     fmt.Errorf("dynamodb throttled"),
		getResponses: map[string]*store.Execution{},
	}
	kubeClient := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()
	cfg := testConfig()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.JobsNamespace}}
	_, _ = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	exec := executor.New(kubeClient, nil, nil, nil, executor.ExecutorConfig{}, noopLogger())
	r := NewReconciler(execStore, kubeClient, nil, exec, cfg, noopLogger())

	err := r.Run(ctx)
	if err == nil {
		t.Fatal("expected error when dispatch phase fails")
	}
	if !contains(err.Error(), "phase errors") {
		t.Errorf("expected 'phase errors' in error message, got: %v", err)
	}
}

func TestRun_WhenNoPhaseFails_ItShouldReturnNil(t *testing.T) {
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

	err := r.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && len(substr) > 0 && indexOfSubstring(s, substr) >= 0
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
