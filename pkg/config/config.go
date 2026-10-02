package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// HandlerMode determines which Lambda handler behavior is active.
	// Valid values: "api", "worker"
	// The "worker" mode handles all scheduled events (reconciler, gc)
	// and async TA execution via self-invocation.
	HandlerMode string

	ExecutionsTable        string
	AuditTable             string
	ArtifactBucket         string
	TargetCluster          string
	DeploymentTarget       string
	Region                 string
	JobImage               string
	WriteCooldownSeconds   int
	MaxConcurrentPerTarget int
	DynamoDBTTLDays        int

	// UploaderRoleARN is the IAM role assumed via STS to generate scoped
	// S3 upload credentials for async K8s Jobs.
	UploaderRoleARN string

	// DataStoreRoleARN is the cross-account IAM role assumed for DynamoDB and S3
	// access when the data layer lives in a different account (MC→RC pattern).
	// If empty, the Lambda uses its own credentials (same-account / RC deployment).
	DataStoreRoleARN string

	// AWSReadRoleARN is the IAM role assumed for read-only AWS TAs (ec2:Describe*, eks:List*, etc.)
	AWSReadRoleARN string
	// AWSWriteRoleARN is the IAM role assumed for write AWS TAs (ec2:*, route53:*, etc.)
	AWSWriteRoleARN string

	// KMSKeyARN is used for DynamoDB/S3 encryption context.
	KMSKeyARN string

	// EKS cluster connection (Lambda is NOT a Pod — cannot use InClusterConfig)
	EKSClusterEndpoint string // Full https:// URL of the EKS API server
	EKSClusterCA       string // Base64-encoded CA certificate
	EKSClusterName     string // Cluster name for token generation (x-k8s-aws-id)

	// --- Code-level deadlines (tunable via env vars without code change) ---

	// ReconcilerDeadlineSeconds is the code-level context.WithTimeout for
	// scheduled routes (reconciler, gc). If this deadline fires,
	// the handler exits cleanly and defers remaining work to the next tick.
	// Must be < Lambda function timeout.
	// Default: 55s. Safe to increase up to (Lambda timeout - 5s).
	ReconcilerDeadlineSeconds int

	// ExecutionDeadlineSeconds is the code-level context.WithTimeout for
	// sync TA execution (api mode) and async TA execution (worker mode).
	// Per-TA TimeoutSeconds takes precedence if set and smaller.
	// Default: 295s. Safe to increase up to (Lambda timeout - 5s).
	ExecutionDeadlineSeconds int

	// MaxBatchPerTick is the max items processed per reconciler/GC tick.
	// Remaining items are deferred to the next tick. Prevents timeout under load.
	// Default: 30.
	MaxBatchPerTick int

	// WorkerFunctionName is the Lambda function name for self-invocation
	// (fan-out from reconciler → TA execution). Read from AWS_LAMBDA_FUNCTION_NAME
	// automatically set by the Lambda runtime.
	WorkerFunctionName string

	// JobsNamespace is the Kubernetes namespace where ZOA Jobs are created.
	JobsNamespace string

	// AsyncSchedulingOverheadSeconds is additional time added to a TA's timeout
	// when checking async execution deadline. Accounts for infrastructure latency:
	// DynamoDB GSI eventual consistency + reconciler cadence (1 min) + Job scheduling.
	// The TA's TimeoutSeconds defines actual execution budget; this overhead is the
	// platform's responsibility and invisible to TA authors.
	// Default: 180s (3 minutes). Tunable without code change.
	AsyncSchedulingOverheadSeconds int

	// --- Access + API mode fields ---

	// SessionsTable is the DynamoDB table for boundary session state.
	// Required in access mode (session lifecycle). Optional in api mode
	// (identity bridge: ECS task ARN → SRE identity).
	SessionsTable string

	// TargetsSSMPrefix is the SSM Parameter Store path prefix for target registry
	// (e.g., /zoa/targets/us-east-1). Each cluster writes its own parameter.
	TargetsSSMPrefix string

	// ECSClusterARN is the default ECS cluster ARN for boundary tasks (RC targets).
	ECSClusterARN string

	// ECSTaskDefinitionARN is the default ECS task definition ARN for boundary containers.
	ECSTaskDefinitionARN string

	// ECSSubnets is a comma-separated list of subnet IDs for boundary ECS tasks.
	ECSSubnets string

	// ECSSecurityGroup is the security group ID for boundary ECS tasks.
	ECSSecurityGroup string

	// BoundaryECSExecCommand is the ecs:ExecuteCommand interactive shell (from Terraform / ZOA_ECS_EXEC_COMMAND).
	BoundaryECSExecCommand string
}

func Load() (*Config, error) {
	cfg := &Config{
		HandlerMode:                    getEnv("HANDLER_MODE", "api"),
		ExecutionsTable:                getEnv("EXECUTION_TABLE", ""),
		AuditTable:                     getEnv("AUDIT_TABLE", ""),
		ArtifactBucket:                 getEnv("ARTIFACT_BUCKET", ""),
		TargetCluster:                  getEnv("TARGET_CLUSTER", ""),
		DeploymentTarget:               getEnv("ZOA_DEPLOYMENT_TARGET", ""),
		Region:                         getEnv("AWS_REGION", "us-east-1"),
		JobImage:                       getEnv("JOB_IMAGE", ""),
		WriteCooldownSeconds:           getEnvInt("WRITE_COOLDOWN_SECONDS", 300),
		MaxConcurrentPerTarget:         getEnvInt("MAX_CONCURRENT_PER_TARGET", 10),
		DynamoDBTTLDays:                getEnvInt("DYNAMODB_TTL_DAYS", 365),
		UploaderRoleARN:                getEnv("UPLOADER_ROLE_ARN", ""),
		DataStoreRoleARN:               getEnv("DATA_STORE_ROLE_ARN", ""),
		AWSReadRoleARN:                 getEnv("AWS_READ_ROLE_ARN", ""),
		AWSWriteRoleARN:                getEnv("AWS_WRITE_ROLE_ARN", ""),
		KMSKeyARN:                      getEnv("KMS_KEY_ARN", ""),
		EKSClusterEndpoint:             getEnv("EKS_CLUSTER_ENDPOINT", ""),
		EKSClusterCA:                   getEnv("EKS_CLUSTER_CA", ""),
		EKSClusterName:                 getEnv("EKS_CLUSTER_NAME", ""),
		ReconcilerDeadlineSeconds:      getEnvInt("RECONCILER_DEADLINE_SECONDS", 55),
		ExecutionDeadlineSeconds:       getEnvInt("EXECUTION_DEADLINE_SECONDS", 295),
		MaxBatchPerTick:                getEnvInt("MAX_BATCH_PER_TICK", 30),
		WorkerFunctionName:             getEnv("AWS_LAMBDA_FUNCTION_NAME", ""),
		JobsNamespace:                  getEnv("ZOA_JOBS_NAMESPACE", "zoa-jobs"),
		AsyncSchedulingOverheadSeconds: getEnvInt("ASYNC_SCHEDULING_OVERHEAD_SECONDS", 180),
		SessionsTable:                  getEnv("SESSIONS_TABLE", ""),
		TargetsSSMPrefix:               getEnv("TARGETS_SSM_PREFIX", ""),
		ECSClusterARN:                  getEnv("ECS_CLUSTER_ARN", ""),
		ECSTaskDefinitionARN:           getEnv("ECS_TASK_DEFINITION_ARN", ""),
		ECSSubnets:                     getEnv("ECS_SUBNETS", ""),
		ECSSecurityGroup:               getEnv("ECS_SECURITY_GROUP", ""),
		BoundaryECSExecCommand:         getEnv("ZOA_ECS_EXEC_COMMAND", "runuser -u sre -- /bin/bash -l"),
	}

	validModes := map[string]bool{"api": true, "worker": true, "access": true}
	if !validModes[cfg.HandlerMode] {
		return nil, fmt.Errorf("invalid HANDLER_MODE %q: must be 'api', 'worker', or 'access'", cfg.HandlerMode)
	}

	// Access mode has a different set of required fields — it does NOT talk to EKS.
	if cfg.IsAccessMode() {
		if cfg.SessionsTable == "" {
			return nil, fmt.Errorf("SESSIONS_TABLE is required in access mode")
		}
		if cfg.TargetsSSMPrefix == "" {
			return nil, fmt.Errorf("TARGETS_SSM_PREFIX is required in access mode")
		}
		if cfg.AuditTable == "" {
			return nil, fmt.Errorf("AUDIT_TABLE is required in access mode")
		}
		if cfg.Region == "" {
			return nil, fmt.Errorf("AWS_REGION is required in access mode")
		}
		return cfg, nil
	}

	// Common validation for api and worker modes.
	if cfg.ExecutionsTable == "" {
		return nil, fmt.Errorf("EXECUTION_TABLE is required")
	}
	if cfg.ArtifactBucket == "" {
		return nil, fmt.Errorf("ARTIFACT_BUCKET is required")
	}
	if cfg.EKSClusterEndpoint == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_ENDPOINT is required (Lambda cannot use InClusterConfig)")
	}
	if cfg.EKSClusterCA == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_CA is required (base64-encoded CA cert)")
	}
	if cfg.EKSClusterName == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_NAME is required (used for EKS token generation)")
	}
	if cfg.DeploymentTarget == "" {
		return nil, fmt.Errorf("ZOA_DEPLOYMENT_TARGET is required (rc or mc)")
	}
	if cfg.DeploymentTarget != "rc" && cfg.DeploymentTarget != "mc" {
		return nil, fmt.Errorf("invalid ZOA_DEPLOYMENT_TARGET %q: must be rc or mc", cfg.DeploymentTarget)
	}

	if cfg.HandlerMode == "api" {
		if cfg.AuditTable == "" {
			return nil, fmt.Errorf("AUDIT_TABLE is required in api mode")
		}
		if cfg.TargetCluster == "" {
			return nil, fmt.Errorf("TARGET_CLUSTER is required in api mode")
		}
	}

	if cfg.HandlerMode == "worker" {
		if cfg.TargetCluster == "" {
			return nil, fmt.Errorf("TARGET_CLUSTER is required in worker mode")
		}
		if cfg.UploaderRoleARN == "" {
			return nil, fmt.Errorf("UPLOADER_ROLE_ARN is required in worker mode (needed for async TA S3 credentials)")
		}
		if cfg.JobImage == "" {
			return nil, fmt.Errorf("JOB_IMAGE is required in worker mode (used for async TA K8s Jobs)")
		}
	}

	if cfg.HandlerMode == "api" {
		if cfg.UploaderRoleARN == "" {
			return nil, fmt.Errorf("UPLOADER_ROLE_ARN is required in api mode (needed for async dispatch from API)")
		}
		if cfg.JobImage == "" {
			return nil, fmt.Errorf("JOB_IMAGE is required in api mode (used for async TA K8s Jobs)")
		}
	}

	return cfg, nil
}

func (c *Config) IsAPIMode() bool {
	return c.HandlerMode == "api"
}

func (c *Config) IsWorkerMode() bool {
	return c.HandlerMode == "worker"
}

func (c *Config) IsAccessMode() bool {
	return c.HandlerMode == "access"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
