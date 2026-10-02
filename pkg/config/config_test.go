package config

import (
	"os"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("EXECUTION_TABLE", "zoa-executions")
	t.Setenv("ARTIFACT_BUCKET", "zoa-artifacts")
	t.Setenv("EKS_CLUSTER_ENDPOINT", "https://ABCDEF.gr7.us-east-1.eks.amazonaws.com")
	t.Setenv("EKS_CLUSTER_CA", "LS0tLS1CRUdJTi...")
	t.Setenv("EKS_CLUSTER_NAME", "eph-test-rc")
	t.Setenv("TARGET_CLUSTER", "eph-test-rc")
	t.Setenv("ZOA_DEPLOYMENT_TARGET", "rc")
	t.Setenv("AUDIT_TABLE", "zoa-audit")
	t.Setenv("UPLOADER_ROLE_ARN", "arn:aws:iam::123456:role/zoa-uploader")
	t.Setenv("JOB_IMAGE", "123456.dkr.ecr.us-east-1.amazonaws.com/zoa-runner:latest")
}

func TestLoad_WhenAPIMode_ItShouldSucceedWithRequiredVars(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "api")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HandlerMode != "api" {
		t.Errorf("expected HandlerMode 'api', got %q", cfg.HandlerMode)
	}
	if cfg.ExecutionsTable != "zoa-executions" {
		t.Errorf("expected ExecutionsTable 'zoa-executions', got %q", cfg.ExecutionsTable)
	}
	if !cfg.IsAPIMode() {
		t.Error("expected IsAPIMode() true")
	}
	if cfg.IsWorkerMode() {
		t.Error("expected IsWorkerMode() false")
	}
}

func TestLoad_WhenWorkerMode_ItShouldSucceedWithRequiredVars(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "worker")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HandlerMode != "worker" {
		t.Errorf("expected HandlerMode 'worker', got %q", cfg.HandlerMode)
	}
	if !cfg.IsWorkerMode() {
		t.Error("expected IsWorkerMode() true")
	}
	if cfg.IsAPIMode() {
		t.Error("expected IsAPIMode() false")
	}
}

func setAccessEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HANDLER_MODE", "access")
	t.Setenv("SESSIONS_TABLE", "zoa-boundary-sessions")
	t.Setenv("TARGETS_SSM_PREFIX", "/zoa/targets/us-east-1")
	t.Setenv("AUDIT_TABLE", "zoa-audit")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("EXEC_SCOPED_ROLE_ARN", "arn:aws:iam::123456789012:role/test-zoa-access-exec-scoped")
}

func TestLoad_WhenAccessMode_ItShouldSucceedWithRequiredVars(t *testing.T) {
	setAccessEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HandlerMode != "access" {
		t.Errorf("expected HandlerMode 'access', got %q", cfg.HandlerMode)
	}
	if !cfg.IsAccessMode() {
		t.Error("expected IsAccessMode() true")
	}
	if cfg.IsAPIMode() {
		t.Error("expected IsAPIMode() false")
	}
	if cfg.IsWorkerMode() {
		t.Error("expected IsWorkerMode() false")
	}
	if cfg.SessionsTable != "zoa-boundary-sessions" {
		t.Errorf("expected SessionsTable 'zoa-boundary-sessions', got %q", cfg.SessionsTable)
	}
	if cfg.TargetsSSMPrefix != "/zoa/targets/us-east-1" {
		t.Errorf("expected TargetsSSMPrefix '/zoa/targets/us-east-1', got %q", cfg.TargetsSSMPrefix)
	}
}

func TestLoad_WhenAccessModeMissingSessionsTable_ItShouldReturnError(t *testing.T) {
	setAccessEnv(t)
	t.Setenv("SESSIONS_TABLE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when SESSIONS_TABLE is empty in access mode")
	}
}

func TestLoad_WhenAccessModeMissingTargetsSSMPrefix_ItShouldReturnError(t *testing.T) {
	setAccessEnv(t)
	t.Setenv("TARGETS_SSM_PREFIX", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when TARGETS_SSM_PREFIX is empty in access mode")
	}
}

func TestLoad_WhenAccessModeMissingAuditTable_ItShouldReturnError(t *testing.T) {
	setAccessEnv(t)
	t.Setenv("AUDIT_TABLE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUDIT_TABLE is empty in access mode")
	}
}

func TestLoad_WhenAccessModeDoesNotRequireEKS_ItShouldSucceed(t *testing.T) {
	setAccessEnv(t)
	// Access mode should NOT require EKS fields
	t.Setenv("EKS_CLUSTER_ENDPOINT", "")
	t.Setenv("EKS_CLUSTER_CA", "")
	t.Setenv("EKS_CLUSTER_NAME", "")
	t.Setenv("EXECUTION_TABLE", "")
	t.Setenv("ARTIFACT_BUCKET", "")
	t.Setenv("ZOA_DEPLOYMENT_TARGET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.IsAccessMode() {
		t.Error("expected IsAccessMode() true")
	}
}

func TestLoad_WhenInvalidMode_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestLoad_WhenMissingExecutionTable_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("EXECUTION_TABLE")
	t.Setenv("EXECUTION_TABLE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when EXECUTION_TABLE is empty")
	}
}

func TestLoad_WhenMissingArtifactBucket_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ARTIFACT_BUCKET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ARTIFACT_BUCKET is empty")
	}
}

func TestLoad_WhenMissingEKSEndpoint_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EKS_CLUSTER_ENDPOINT", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when EKS_CLUSTER_ENDPOINT is empty")
	}
}

func TestLoad_WhenMissingEKSCA_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EKS_CLUSTER_CA", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when EKS_CLUSTER_CA is empty")
	}
}

func TestLoad_WhenMissingEKSClusterName_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EKS_CLUSTER_NAME", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when EKS_CLUSTER_NAME is empty")
	}
}

func TestLoad_WhenAPIMissingAuditTable_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "api")
	t.Setenv("AUDIT_TABLE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when AUDIT_TABLE is empty in api mode")
	}
}

func TestLoad_WhenAPIMissingTargetCluster_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "api")
	t.Setenv("TARGET_CLUSTER", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when TARGET_CLUSTER is empty in api mode")
	}
}

func TestLoad_WhenWorkerMissingUploaderRole_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "worker")
	t.Setenv("UPLOADER_ROLE_ARN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when UPLOADER_ROLE_ARN is empty in worker mode")
	}
}

func TestLoad_WhenWorkerMissingJobImage_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HANDLER_MODE", "worker")
	t.Setenv("JOB_IMAGE", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when JOB_IMAGE is empty in worker mode")
	}
}

func TestLoad_WhenMissingDeploymentTarget_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ZOA_DEPLOYMENT_TARGET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ZOA_DEPLOYMENT_TARGET is empty")
	}
}

func TestLoad_WhenInvalidDeploymentTarget_ItShouldReturnError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ZOA_DEPLOYMENT_TARGET", "hcp")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid ZOA_DEPLOYMENT_TARGET")
	}
}

func TestLoad_WhenDefaultValues_ItShouldApplyDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WriteCooldownSeconds != 300 {
		t.Errorf("expected WriteCooldownSeconds=300, got %d", cfg.WriteCooldownSeconds)
	}
	if cfg.MaxConcurrentPerTarget != 10 {
		t.Errorf("expected MaxConcurrentPerTarget=10, got %d", cfg.MaxConcurrentPerTarget)
	}
	if cfg.DynamoDBTTLDays != 365 {
		t.Errorf("expected DynamoDBTTLDays=365, got %d", cfg.DynamoDBTTLDays)
	}
	if cfg.ReconcilerDeadlineSeconds != 55 {
		t.Errorf("expected ReconcilerDeadlineSeconds=55, got %d", cfg.ReconcilerDeadlineSeconds)
	}
	if cfg.ExecutionDeadlineSeconds != 295 {
		t.Errorf("expected ExecutionDeadlineSeconds=295, got %d", cfg.ExecutionDeadlineSeconds)
	}
	if cfg.MaxBatchPerTick != 30 {
		t.Errorf("expected MaxBatchPerTick=30, got %d", cfg.MaxBatchPerTick)
	}
	if cfg.JobsNamespace != "zoa-jobs" {
		t.Errorf("expected JobsNamespace='zoa-jobs', got %q", cfg.JobsNamespace)
	}
	if cfg.AsyncSchedulingOverheadSeconds != 180 {
		t.Errorf("expected AsyncSchedulingOverheadSeconds=180, got %d", cfg.AsyncSchedulingOverheadSeconds)
	}
}

func TestLoad_WhenCustomIntEnvVars_ItShouldOverrideDefaults(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WRITE_COOLDOWN_SECONDS", "60")
	t.Setenv("MAX_CONCURRENT_PER_TARGET", "5")
	t.Setenv("RECONCILER_DEADLINE_SECONDS", "45")
	t.Setenv("EXECUTION_DEADLINE_SECONDS", "120")
	t.Setenv("MAX_BATCH_PER_TICK", "50")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WriteCooldownSeconds != 60 {
		t.Errorf("expected WriteCooldownSeconds=60, got %d", cfg.WriteCooldownSeconds)
	}
	if cfg.MaxConcurrentPerTarget != 5 {
		t.Errorf("expected MaxConcurrentPerTarget=5, got %d", cfg.MaxConcurrentPerTarget)
	}
	if cfg.ReconcilerDeadlineSeconds != 45 {
		t.Errorf("expected ReconcilerDeadlineSeconds=45, got %d", cfg.ReconcilerDeadlineSeconds)
	}
	if cfg.ExecutionDeadlineSeconds != 120 {
		t.Errorf("expected ExecutionDeadlineSeconds=120, got %d", cfg.ExecutionDeadlineSeconds)
	}
	if cfg.MaxBatchPerTick != 50 {
		t.Errorf("expected MaxBatchPerTick=50, got %d", cfg.MaxBatchPerTick)
	}
}

func TestLoad_WhenInvalidIntEnvVar_ItShouldUseFallback(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WRITE_COOLDOWN_SECONDS", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WriteCooldownSeconds != 300 {
		t.Errorf("expected fallback WriteCooldownSeconds=300, got %d", cfg.WriteCooldownSeconds)
	}
}

func TestGetEnv_WhenSet_ItShouldReturnValue(t *testing.T) {
	t.Setenv("TEST_VAR_XYZ", "hello")
	result := getEnv("TEST_VAR_XYZ", "default")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestGetEnv_WhenNotSet_ItShouldReturnFallback(t *testing.T) {
	os.Unsetenv("TEST_VAR_NOT_EXISTS")
	result := getEnv("TEST_VAR_NOT_EXISTS", "fallback")
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %q", result)
	}
}

func TestGetEnvInt_WhenValid_ItShouldReturnParsedInt(t *testing.T) {
	t.Setenv("TEST_INT_VAR", "42")
	result := getEnvInt("TEST_INT_VAR", 0)
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestGetEnvInt_WhenInvalid_ItShouldReturnFallback(t *testing.T) {
	t.Setenv("TEST_INT_VAR", "abc")
	result := getEnvInt("TEST_INT_VAR", 99)
	if result != 99 {
		t.Errorf("expected fallback 99, got %d", result)
	}
}

func TestGetEnvInt_WhenEmpty_ItShouldReturnFallback(t *testing.T) {
	os.Unsetenv("TEST_INT_EMPTY")
	result := getEnvInt("TEST_INT_EMPTY", 7)
	if result != 7 {
		t.Errorf("expected fallback 7, got %d", result)
	}
}
