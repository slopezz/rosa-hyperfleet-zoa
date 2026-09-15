package executor

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

type mockSTSClient struct {
	output *sts.AssumeRoleOutput
	err    error
	calls  []sts.AssumeRoleInput
}

func (m *mockSTSClient) AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, _ ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	m.calls = append(m.calls, *params)
	return m.output, m.err
}

func newTestExecutor(client *fake.Clientset, stsClient STSAssumeRoler) *Executor {
	return &Executor{
		kubeClient:      client,
		restConfig:      &rest.Config{Host: "https://localhost:6443"},
		stsClient:       stsClient,
		artifactBucket:  "test-bucket",
		uploaderRoleARN: "arn:aws:iam::123456789012:role/zoa-uploader",
		region:          "us-east-1",
		jobImage:        "quay.io/openshift/zoa-tools:latest",
		logger:          noopLogger(),
		eksCircuit:      newCircuitBreaker("test-cluster"),
	}
}

func TestDispatchAsync_WhenCalled_ItShouldCreateSTSScopedSecret(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{ // notsecret — fake STS credentials for unit tests
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIAEXAMPLE"), // notsecret
				SecretAccessKey: aws.String("secret123"),   // notsecret
				SessionToken:    aws.String("token456"),    // notsecret
			},
		},
	}

	e := newTestExecutor(client, mockSTS)

	exec := &store.Execution{
		ID:            "abcd1234-5678-9012-3456-789012345678",
		Action:        "delete-pod",
		TargetCluster: "test-cluster",
		Operator:      "sre@redhat.com",
		Params:        map[string]string{"namespace": "default", "name": "test-pod"},
	}

	action := &fakeAction{
		meta: actions.ActionMetadata{
			Name:           "delete-pod",
			TimeoutSeconds: 120,
			Scope:          "cluster",
			RBAC: &actions.RBACConfig{
				ClusterScoped: false,
				Rules: []actions.RBACRule{
					{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"delete"}},
				},
			},
		},
	}

	err := e.DispatchAsync(ctx, exec, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify STS was called with correct parameters
	if len(mockSTS.calls) != 1 {
		t.Fatalf("expected 1 STS call, got %d", len(mockSTS.calls))
	}
	stsCall := mockSTS.calls[0]
	if *stsCall.RoleArn != "arn:aws:iam::123456789012:role/zoa-uploader" {
		t.Errorf("wrong role ARN: %s", *stsCall.RoleArn)
	}
	if *stsCall.DurationSeconds != 3600 {
		t.Errorf("expected 3600s duration, got %d", *stsCall.DurationSeconds)
	}
	if stsCall.Policy == nil || len(*stsCall.Policy) == 0 {
		t.Fatal("session policy should not be empty")
	}

	// Verify Secret was created with STS credentials
	secretName := fmt.Sprintf("zoa-creds-%s", exec.ID)
	secret, err := client.CoreV1().Secrets(jobNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("credentials secret not found: %v", err)
	}
	if string(secret.Data["AWS_ACCESS_KEY_ID"]) != "AKIAEXAMPLE" { // notsecret
		t.Errorf("wrong access key in secret: %s", string(secret.Data["AWS_ACCESS_KEY_ID"]))
	}
	if string(secret.Data["AWS_SECRET_ACCESS_KEY"]) != "secret123" { // notsecret
		t.Errorf("wrong secret key in secret")
	}
	if string(secret.Data["AWS_SESSION_TOKEN"]) != "token456" { // notsecret
		t.Errorf("wrong session token in secret")
	}
	if string(secret.Data["AWS_DEFAULT_REGION"]) != "us-east-1" {
		t.Errorf("wrong region in secret: %s", string(secret.Data["AWS_DEFAULT_REGION"]))
	}
	if secret.Labels[labelKey] != exec.ID {
		t.Errorf("secret missing execution label")
	}
}

func TestDispatchAsync_WhenCalled_ItShouldCreateJobWithSecretEnvFrom(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA"),
				SecretAccessKey: aws.String("secret"),
				SessionToken:    aws.String("token"),
			},
		},
	}

	e := newTestExecutor(client, mockSTS)

	exec := &store.Execution{
		ID:            "job-test-1234-5678-9012-345678901234",
		Action:        "get-resource",
		TargetCluster: "mc-01",
		Operator:      "sre@redhat.com",
		Params:        map[string]string{"resource": "pods"},
	}

	action := &fakeAction{
		meta: actions.ActionMetadata{
			Name:           "get-resource",
			TimeoutSeconds: 60,
			Scope:          "cluster",
			RBAC: &actions.RBACConfig{
				ClusterScoped: true,
				Rules: []actions.RBACRule{
					{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
				},
			},
		},
	}

	err := e.DispatchAsync(ctx, exec, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Job structure
	jobName := fmt.Sprintf("zoa-%s", exec.ID)
	job, err := client.BatchV1().Jobs(jobNamespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("job not found: %v", err)
	}

	// Job deadline = TA timeout + 300s upload buffer
	expectedDeadline := int64(60 + 300)
	if *job.Spec.ActiveDeadlineSeconds != expectedDeadline {
		t.Errorf("expected deadline %d, got %d", expectedDeadline, *job.Spec.ActiveDeadlineSeconds)
	}

	pod := job.Spec.Template.Spec
	if pod.ServiceAccountName != fmt.Sprintf("zoa-exec-%s", exec.ID) {
		t.Errorf("wrong service account: %s", pod.ServiceAccountName)
	}

	if len(pod.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Containers))
	}
	container := pod.Containers[0]

	// EnvFrom references the credentials Secret
	if len(container.EnvFrom) != 1 {
		t.Fatalf("expected 1 EnvFrom source, got %d", len(container.EnvFrom))
	}
	expectedSecretName := fmt.Sprintf("zoa-creds-%s", exec.ID)
	if container.EnvFrom[0].SecretRef.Name != expectedSecretName {
		t.Errorf("wrong secret ref: %s", container.EnvFrom[0].SecretRef.Name)
	}

	// Env vars for TA execution
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}
	if envMap["EXECUTION_ID"] != exec.ID {
		t.Errorf("wrong EXECUTION_ID env: %s", envMap["EXECUTION_ID"])
	}
	if envMap["ACTION"] != exec.Action {
		t.Errorf("wrong ACTION env: %s", envMap["ACTION"])
	}
	if envMap["ARTIFACT_BUCKET"] != "test-bucket" {
		t.Errorf("wrong ARTIFACT_BUCKET env: %s", envMap["ARTIFACT_BUCKET"])
	}
	expectedPrefix := fmt.Sprintf("executions/%s", exec.ID)
	if envMap["S3_PREFIX"] != expectedPrefix {
		t.Errorf("wrong S3_PREFIX env: %s", envMap["S3_PREFIX"])
	}
	if envMap["OPERATOR"] != exec.Operator {
		t.Errorf("wrong OPERATOR env: %s", envMap["OPERATOR"])
	}

	// Volume mount for output workspace
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/output" {
		t.Errorf("expected /output volume mount, got %v", container.VolumeMounts)
	}

	// EmptyDir volume exists
	if len(pod.Volumes) != 1 || pod.Volumes[0].EmptyDir == nil {
		t.Error("expected emptyDir volume for output")
	}
}

func TestDispatchAsync_WhenSTSFails_ItShouldCleanupAndReturnError(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		err: fmt.Errorf("access denied: not authorized to assume role"),
	}

	e := newTestExecutor(client, mockSTS)

	exec := &store.Execution{
		ID:            "fail-sts-1234-5678-9012-345678901234",
		Action:        "delete-pod",
		TargetCluster: "mc-01",
		Params:        map[string]string{},
	}

	action := &fakeAction{
		meta: actions.ActionMetadata{
			Name:  "delete-pod",
			Scope: "cluster",
			RBAC: &actions.RBACConfig{
				ClusterScoped: false,
				Rules:         []actions.RBACRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"delete"}}},
			},
		},
	}

	err := e.DispatchAsync(ctx, exec, action)
	if err == nil {
		t.Fatal("expected error when STS fails")
	}

	// SA should have been cleaned up
	saName := fmt.Sprintf("zoa-exec-%s", exec.ID)
	_, saErr := client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, saName, metav1.GetOptions{})
	if saErr == nil {
		t.Error("expected SA to be cleaned up after STS failure")
	}
}

func TestDispatchAsync_WhenSessionPolicy_ItShouldScopeToExecutionPrefix(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA"),
				SecretAccessKey: aws.String("s"),
				SessionToken:    aws.String("t"),
			},
		},
	}

	e := newTestExecutor(client, mockSTS)

	exec := &store.Execution{
		ID:     "scope-1234-5678-9012-345678901234",
		Action: "get-resource",
		Params: map[string]string{},
	}
	action := &fakeAction{meta: actions.ActionMetadata{Name: "get-resource", Scope: "cluster"}}

	_ = e.DispatchAsync(ctx, exec, action)

	policy := *mockSTS.calls[0].Policy
	expectedResource := fmt.Sprintf("arn:aws:s3:::test-bucket/executions/%s/*", exec.ID)
	if !contains(policy, expectedResource) {
		t.Errorf("session policy should scope to execution prefix.\nPolicy: %s\nExpected to contain: %s", policy, expectedResource)
	}
}

func TestDispatchAsync_WhenKubeAPIScope_ItShouldIncludeKMSInSessionPolicy(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA"),
				SecretAccessKey: aws.String("s"),
				SessionToken:    aws.String("t"),
			},
		},
	}

	e := newTestExecutor(client, mockSTS)
	e.kmsKeyARN = "arn:aws:kms:us-east-1:123456789012:key/test-key-id"

	exec := &store.Execution{
		ID:     "kms-test-1234-5678-9012-345678901234",
		Action: "get-resource",
		Params: map[string]string{},
	}
	action := &fakeAction{meta: actions.ActionMetadata{
		Name:  "get-resource",
		Scope: "kube-api",
		Type:  "read",
		RBAC:  &actions.RBACConfig{ClusterScoped: true, Rules: []actions.RBACRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}},
	}}

	err := e.DispatchAsync(ctx, exec, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSTS.calls) != 1 {
		t.Fatalf("expected 1 STS call, got %d", len(mockSTS.calls))
	}
	stsCall := mockSTS.calls[0]

	if stsCall.Policy == nil {
		t.Fatal("kube-api TA should have a session policy")
	}
	if !contains(*stsCall.Policy, "kms:GenerateDataKey") {
		t.Errorf("session policy should include kms:GenerateDataKey, got: %s", *stsCall.Policy)
	}
	if !contains(*stsCall.Policy, "arn:aws:kms:us-east-1:123456789012:key/test-key-id") {
		t.Errorf("session policy should reference the KMS key ARN, got: %s", *stsCall.Policy)
	}
}

func TestDispatchAsync_WhenAWSAPIScope_ItShouldNotIncludeSessionPolicy(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA"),
				SecretAccessKey: aws.String("s"),
				SessionToken:    aws.String("t"),
			},
		},
	}

	e := newTestExecutor(client, mockSTS)
	e.awsReadRoleARN = "arn:aws:iam::123456789012:role/zoa-aws-read"

	exec := &store.Execution{
		ID:            "aws-api-1234-5678-9012-345678901234",
		Action:        "describe_vpc_endpoint",
		TargetCluster: "test-cluster",
		Params:        map[string]string{"name": "vpce-123"},
	}
	action := &fakeAction{meta: actions.ActionMetadata{
		Name:  "describe_vpc_endpoint",
		Scope: "aws-api",
		Type:  "read",
	}}

	err := e.DispatchAsync(ctx, exec, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockSTS.calls) != 1 {
		t.Fatalf("expected 1 STS call, got %d", len(mockSTS.calls))
	}
	stsCall := mockSTS.calls[0]

	// aws-api TAs must NOT have a session policy (it would strip AWS API permissions)
	if stsCall.Policy != nil {
		t.Errorf("aws-api TA should NOT have session policy, but got: %s", *stsCall.Policy)
	}

	// Should use the aws-read role, not the uploader role
	if *stsCall.RoleArn != "arn:aws:iam::123456789012:role/zoa-aws-read" {
		t.Errorf("expected aws-read role, got %s", *stsCall.RoleArn)
	}
}

func TestDispatchAsync_WhenAWSAPIWriteScope_ItShouldUseWriteRole(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA"),
				SecretAccessKey: aws.String("s"),
				SessionToken:    aws.String("t"),
			},
		},
	}

	e := newTestExecutor(client, mockSTS)
	e.awsReadRoleARN = "arn:aws:iam::123456789012:role/zoa-aws-read"
	e.awsWriteRoleARN = "arn:aws:iam::123456789012:role/zoa-aws-write"

	exec := &store.Execution{
		ID:            "aws-wr-1234-5678-9012-345678901234",
		Action:        "modify_vpc_endpoint",
		TargetCluster: "test-cluster",
		Params:        map[string]string{"name": "vpce-123"},
	}
	action := &fakeAction{meta: actions.ActionMetadata{
		Name:  "modify_vpc_endpoint",
		Scope: "aws-api",
		Type:  "write",
	}}

	err := e.DispatchAsync(ctx, exec, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stsCall := mockSTS.calls[0]
	if *stsCall.RoleArn != "arn:aws:iam::123456789012:role/zoa-aws-write" {
		t.Errorf("expected aws-write role for write TAs, got %s", *stsCall.RoleArn)
	}
	if stsCall.Policy != nil {
		t.Errorf("aws-api write TA should NOT have session policy")
	}
}

func TestDispatchAsync_WhenAWSAPIScope_ItShouldSkipKubeRBAC(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	mockSTS := &mockSTSClient{
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA"),
				SecretAccessKey: aws.String("s"),
				SessionToken:    aws.String("t"),
			},
		},
	}

	e := newTestExecutor(client, mockSTS)
	e.awsReadRoleARN = "arn:aws:iam::123456789012:role/zoa-aws-read"

	exec := &store.Execution{
		ID:            "no-rbac-1234-5678-9012-345678901234",
		Action:        "describe_vpc",
		TargetCluster: "test-cluster",
		Params:        map[string]string{},
	}
	action := &fakeAction{meta: actions.ActionMetadata{
		Name:  "describe_vpc",
		Scope: "aws-api",
		Type:  "read",
		RBAC: &actions.RBACConfig{
			ClusterScoped: false,
			Rules: []actions.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
			},
		},
	}}

	err := e.DispatchAsync(ctx, exec, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SA should exist but NO Role/RoleBinding should be created
	saName := fmt.Sprintf("zoa-exec-%s", exec.ID)
	_, err = client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, saName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("SA should exist: %v", err)
	}

	// Roles should NOT be created for aws-api TAs
	roles, _ := client.RbacV1().Roles(jobNamespace).List(ctx, metav1.ListOptions{})
	if len(roles.Items) > 0 {
		t.Errorf("aws-api TAs should not create K8s RBAC roles, found %d", len(roles.Items))
	}
	clusterRoles, _ := client.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if len(clusterRoles.Items) > 0 {
		t.Errorf("aws-api TAs should not create K8s ClusterRoles, found %d", len(clusterRoles.Items))
	}
}

func TestCleanupResources_WhenAsyncExecution_ItShouldDeleteCredentialsSecret(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	// Pre-create the credentials secret (as DispatchAsync would)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zoa-creds-exec-cleanup-test",
			Namespace: jobNamespace,
		},
		StringData: map[string]string{ // notsecret — test fixture, not real credentials
			"AWS_ACCESS_KEY_ID": "AKIA",
		},
	}
	_, _ = client.CoreV1().Secrets(jobNamespace).Create(ctx, secret, metav1.CreateOptions{})

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "zoa-exec-exec-cleanup-test", Namespace: jobNamespace}}
	_, _ = client.CoreV1().ServiceAccounts(jobNamespace).Create(ctx, sa, metav1.CreateOptions{})

	e := &Executor{kubeClient: client, logger: noopLogger()}
	e.cleanupResources(ctx, "exec-cleanup-test", "zoa-exec-exec-cleanup-test", nil, nil)

	// Secret should be gone
	_, err := client.CoreV1().Secrets(jobNamespace).Get(ctx, "zoa-creds-exec-cleanup-test", metav1.GetOptions{})
	if err == nil {
		t.Error("expected credentials secret to be deleted after cleanup")
	}

	// SA should be gone
	_, err = client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, "zoa-exec-exec-cleanup-test", metav1.GetOptions{})
	if err == nil {
		t.Error("expected service account to be deleted after cleanup")
	}
}

func TestClientForServiceAccount_WhenCalled_ItShouldReturnImpersonatingClient(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{
		kubeClient: client,
		restConfig: &rest.Config{Host: "https://localhost:6443"},
		logger:     noopLogger(),
	}

	impersonatedClient, dynClient, _, err := e.clientForServiceAccount(context.Background(), "zoa-exec-test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if impersonatedClient == nil {
		t.Fatal("expected non-nil client")
	}
	if dynClient == nil {
		t.Fatal("expected non-nil dynamic client")
	}
}

// fakeAction implements actions.Action for testing
type fakeAction struct {
	meta   actions.ActionMetadata
	result *actions.ActionResult
	err    error
}

func (f *fakeAction) Metadata() actions.ActionMetadata {
	return f.meta
}

func (f *fakeAction) Validate(_ context.Context, _ *actions.ExecutionParams) error {
	return nil
}

func (f *fakeAction) Execute(_ context.Context, _ *actions.ExecutionParams) (*actions.ActionResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &actions.ActionResult{Success: true}, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
