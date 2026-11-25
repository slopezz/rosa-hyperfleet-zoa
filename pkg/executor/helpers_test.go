package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

// --- isTransientError tests ---

func TestIsTransientError_WhenServerTimeout_ItShouldReturnTrue(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonServerTimeout}}
	if !isTransientError(err) {
		t.Error("expected ServerTimeout to be transient")
	}
}

func TestIsTransientError_WhenTooManyRequests_ItShouldReturnTrue(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonTooManyRequests}}
	if !isTransientError(err) {
		t.Error("expected TooManyRequests to be transient")
	}
}

func TestIsTransientError_WhenServiceUnavailable_ItShouldReturnTrue(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonServiceUnavailable}}
	if !isTransientError(err) {
		t.Error("expected ServiceUnavailable to be transient")
	}
}

func TestIsTransientError_WhenInternalError_ItShouldReturnTrue(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonInternalError}}
	if !isTransientError(err) {
		t.Error("expected InternalError to be transient")
	}
}

func TestIsTransientError_WhenNotFound_ItShouldReturnFalse(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}
	if isTransientError(err) {
		t.Error("expected NotFound to NOT be transient")
	}
}

func TestIsTransientError_WhenForbidden_ItShouldReturnFalse(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonForbidden}}
	if isTransientError(err) {
		t.Error("expected Forbidden to NOT be transient")
	}
}

func TestIsTransientError_WhenUnknownReason_ItShouldReturnTrue(t *testing.T) {
	err := &errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonUnknown}}
	if !isTransientError(err) {
		t.Error("expected Unknown reason to be transient (catch-all for network errors)")
	}
}

// --- MarshalActionOutput tests ---
// These tests enforce the critical contract: output.json must contain ONLY
// result.Output (not the full ActionResult envelope). Both sync and async
// paths rely on this for consistent CLI rendering.

func TestMarshalActionOutput_WhenArrayOutput_ItShouldNotIncludeEnvelope(t *testing.T) {
	result := &actions.ActionResult{
		Success: true,
		Output:  []map[string]interface{}{{"Name": "pod-1", "Status": "Running"}},
		Summary: "Found 1 pod",
	}
	data := MarshalActionOutput(result)
	s := string(data)
	if !strings.Contains(s, `"Name"`) {
		t.Errorf("expected output to contain Name field, got: %s", s)
	}
	if strings.Contains(s, `"success"`) {
		t.Errorf("output must NOT contain ActionResult envelope field 'success': %s", s)
	}
	if strings.Contains(s, `"summary"`) {
		t.Errorf("output must NOT contain ActionResult envelope field 'summary': %s", s)
	}
}

func TestMarshalActionOutput_WhenNilOutput_ItShouldReturnEmptyObject(t *testing.T) {
	result := &actions.ActionResult{Success: false, Output: nil, Summary: "execution failed"}
	data := MarshalActionOutput(result)
	if string(data) != "{}" {
		t.Errorf("expected '{}' for nil output, got %q", string(data))
	}
}

func TestMarshalActionOutput_WhenNilResult_ItShouldReturnEmptyObject(t *testing.T) {
	data := MarshalActionOutput(nil)
	if string(data) != "{}" {
		t.Errorf("expected '{}' for nil result, got %q", string(data))
	}
}

func TestMarshalActionOutput_WhenMapOutput_ItShouldSerializeDirectly(t *testing.T) {
	result := &actions.ActionResult{
		Success: true,
		Output:  map[string]interface{}{"name": "argocd-secret", "namespace": "argocd"},
		Summary: "Retrieved secret",
	}
	data := MarshalActionOutput(result)
	s := string(data)
	if !strings.Contains(s, `"name":"argocd-secret"`) {
		t.Errorf("expected direct map serialization, got: %s", s)
	}
	if strings.Contains(s, `"success"`) || strings.Contains(s, `"summary"`) {
		t.Errorf("output must NOT contain envelope fields: %s", s)
	}
}

// --- buildAWSParams tests ---

type mockSTS struct {
	output *sts.AssumeRoleOutput
	err    error
}

func (m *mockSTS) AssumeRole(_ context.Context, _ *sts.AssumeRoleInput, _ ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testLogger() *slog.Logger {
	return noopLogger()
}

func TestBuildAWSParams_WhenReadAction_ItShouldUseReadRole(t *testing.T) {
	mock := &mockSTS{ // notsecret — fake STS credentials for unit tests
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA..."), // notsecret
				SecretAccessKey: aws.String("secret"),  // notsecret
				SessionToken:    aws.String("token"),   // notsecret
			},
		},
	}

	e := &Executor{
		stsClient:       mock,
		awsReadRoleARN:  "arn:aws:iam::123:role/read",
		awsWriteRoleARN: "arn:aws:iam::123:role/write",
		region:          "us-east-1",
		logger:          testLogger(),
	}

	meta := actions.ActionMetadata{Scope: "aws", Type: "read"}
	params, err := e.buildAWSParams(context.Background(), "exec-1", map[string]string{}, meta, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.AWSConfig == nil {
		t.Fatal("expected non-nil AWS config")
	}
}

func TestBuildAWSParams_WhenWriteAction_ItShouldUseWriteRole(t *testing.T) {
	var capturedRoleARN string
	mock := &mockSTS{ // notsecret — fake STS credentials for unit tests
		output: &sts.AssumeRoleOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("AKIA..."), // notsecret
				SecretAccessKey: aws.String("secret"),  // notsecret
				SessionToken:    aws.String("token"),   // notsecret
			},
		},
	}
	// Wrap to capture role
	wrapper := &stsCapture{inner: mock, captured: &capturedRoleARN}

	e := &Executor{
		stsClient:       wrapper,
		awsReadRoleARN:  "arn:aws:iam::123:role/read",
		awsWriteRoleARN: "arn:aws:iam::123:role/write",
		region:          "us-east-1",
		logger:          testLogger(),
	}

	meta := actions.ActionMetadata{Scope: "aws", Type: "write"}
	_, err := e.buildAWSParams(context.Background(), "exec-2", map[string]string{}, meta, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRoleARN != "arn:aws:iam::123:role/write" {
		t.Errorf("expected write role, got %q", capturedRoleARN)
	}
}

type stsCapture struct {
	inner    *mockSTS
	captured *string
}

func (s *stsCapture) AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	*s.captured = *params.RoleArn
	return s.inner.AssumeRole(ctx, params, optFns...)
}

func TestBuildAWSParams_WhenNoSTSClient_ItShouldReturnError(t *testing.T) {
	e := &Executor{
		stsClient: nil,
		logger:    testLogger(),
	}

	meta := actions.ActionMetadata{Scope: "aws", Type: "read"}
	_, err := e.buildAWSParams(context.Background(), "exec-3", map[string]string{}, meta, testLogger())
	if err == nil {
		t.Fatal("expected error when STS client is nil")
	}
}

func TestBuildAWSParams_WhenNoRoleConfigured_ItShouldReturnError(t *testing.T) {
	e := &Executor{
		stsClient:       &mockSTS{},
		awsReadRoleARN:  "", // empty
		awsWriteRoleARN: "",
		logger:          testLogger(),
	}

	meta := actions.ActionMetadata{Scope: "aws", Type: "read"}
	_, err := e.buildAWSParams(context.Background(), "exec-4", map[string]string{}, meta, testLogger())
	if err == nil {
		t.Fatal("expected error when role ARN is empty")
	}
}

func TestBuildAWSParams_WhenSTSFails_ItShouldReturnError(t *testing.T) {
	mock := &mockSTS{err: fmt.Errorf("access denied")}

	e := &Executor{
		stsClient:       mock,
		awsReadRoleARN:  "arn:aws:iam::123:role/read",
		awsWriteRoleARN: "arn:aws:iam::123:role/write",
		region:          "us-east-1",
		logger:          testLogger(),
	}

	meta := actions.ActionMetadata{Scope: "aws", Type: "read"}
	_, err := e.buildAWSParams(context.Background(), "exec-5", map[string]string{}, meta, testLogger())
	if err == nil {
		t.Fatal("expected error when STS AssumeRole fails")
	}
}
