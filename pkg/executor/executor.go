package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

const labelKey = "zoa.openshift.io/execution-id"

var jobNamespace = envOrDefault("ZOA_JOBS_NAMESPACE", "zoa-jobs")

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// STSAssumeRoler abstracts STS AssumeRole for testability.
type STSAssumeRoler interface {
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

type SyncResult struct {
	Success     bool
	Output      json.RawMessage
	Logs        string
	OutputBytes int64
	LogBytes    int64
}

type Executor struct {
	kubeClient      kubernetes.Interface
	restConfig      *rest.Config
	s3Client        S3API
	stsClient       STSAssumeRoler
	awsCfg          *aws.Config
	artifactBucket  string
	uploaderRoleARN string
	awsReadRoleARN  string
	awsWriteRoleARN string
	kmsKeyARN       string
	region          string
	jobImage        string
	logger          *slog.Logger
	eksCircuit      *circuitBreaker
}

type ExecutorConfig struct {
	ArtifactBucket  string
	UploaderRoleARN string
	AWSReadRoleARN  string
	AWSWriteRoleARN string
	KMSKeyARN       string
	Region          string
	JobImage        string
}

func New(kubeClient kubernetes.Interface, restConfig *rest.Config, s3Client S3API, awsCfg *aws.Config, cfg ExecutorConfig, logger *slog.Logger) *Executor {
	var stsClient STSAssumeRoler
	if awsCfg != nil {
		stsClient = sts.NewFromConfig(*awsCfg)
	}
	return &Executor{
		kubeClient:      kubeClient,
		restConfig:      restConfig,
		s3Client:        s3Client,
		stsClient:       stsClient,
		awsCfg:          awsCfg,
		artifactBucket:  cfg.ArtifactBucket,
		uploaderRoleARN: cfg.UploaderRoleARN,
		awsReadRoleARN:  cfg.AWSReadRoleARN,
		awsWriteRoleARN: cfg.AWSWriteRoleARN,
		kmsKeyARN:       cfg.KMSKeyARN,
		region:          cfg.Region,
		jobImage:        cfg.JobImage,
		logger:          logger,
		eksCircuit:      newCircuitBreaker(),
	}
}

// SyncContext provides execution metadata for logging and behavior control.
type SyncContext struct {
	Operator      string
	TargetCluster string
	Force         bool
}

// ExecuteSync runs a TA directly inside the Lambda process via SA impersonation.
// Guarantees: output and logs are always uploaded to S3, even on panic.
func (e *Executor) ExecuteSync(ctx context.Context, executionID string, action actions.Action, params map[string]string, syncCtx *SyncContext) (result *SyncResult, retErr error) {
	if e.kubeClient == nil && e.s3Client == nil {
		return &SyncResult{Success: false}, fmt.Errorf("executor not fully initialized (missing K8s/S3 clients)")
	}
	meta := action.Metadata()
	logger := e.logger.With("execution_id", executionID, "action", meta.Name, "mode", "sync")

	var logBuf bytes.Buffer
	logHandler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	execLogger := slog.New(logHandler)

	operator := ""
	target := ""
	if syncCtx != nil {
		operator = syncCtx.Operator
		target = syncCtx.TargetCluster
	}

	execLogger.Info("sync execution starting",
		"execution_id", executionID,
		"action", meta.Name,
		"target", target,
		"operator", operator,
		"scope", meta.Scope,
		"type", meta.Type,
		"revision", version.GitCommit,
		"params", params,
	)

	var actionResult *actions.ActionResult
	var actionErr error
	var outputBytes []byte

	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			execLogger.Error("PANIC recovered", "panic", fmt.Sprintf("%v", r), "stack", stack)
			actionErr = fmt.Errorf("panic: %v", r)
		}

		logBytes := logBuf.Bytes()
		outputLen, logLen := e.uploadSyncArtifacts(ctx, executionID, outputBytes, logBytes, logger)

		var inlineOutput json.RawMessage
		if len(outputBytes) > 0 && json.Valid(outputBytes) {
			inlineOutput = json.RawMessage(outputBytes)
		} else if len(outputBytes) > 0 {
			inlineOutput, _ = json.Marshal(string(outputBytes))
		}

		result = &SyncResult{
			Success:     actionErr == nil && actionResult != nil && actionResult.Success,
			Output:      inlineOutput,
			Logs:        string(logBytes),
			OutputBytes: outputLen,
			LogBytes:    logLen,
		}
		if actionErr != nil {
			retErr = actionErr
		}
	}()

	var execParams *actions.ExecutionParams

	if meta.Scope == "aws-api" {
		var err error
		execParams, err = e.buildAWSParams(ctx, executionID, params, meta, execLogger)
		if err != nil {
			actionErr = err
			execLogger.Error("failed to build AWS execution params", "error", err)
			outputBytes = marshalErrorOutput(err)
			return
		}
	} else {
		var err error
		execParams, err = e.buildKubeParams(ctx, executionID, params, meta, execLogger)
		if err != nil {
			actionErr = err
			execLogger.Error("failed to build K8s execution params", "error", err)
			outputBytes = marshalErrorOutput(err)
			return
		}
		defer func() {
			logger.Info("cleaning up sync execution resources")
			saName := fmt.Sprintf("zoa-exec-%s", executionID)
			e.cleanupResources(ctx, executionID, saName, meta.RBAC, params)
		}()
	}

	if syncCtx != nil && syncCtx.Force {
		execParams.Force = true
	}

	if err := action.Validate(ctx, execParams); err != nil {
		actionErr = fmt.Errorf("validation failed: %w", err)
		execLogger.Error("action validation failed", "error", err)
		outputBytes = marshalErrorOutput(actionErr)
		return
	}

	actionResult, actionErr = action.Execute(ctx, execParams)

	if actionErr != nil {
		execLogger.Error("execution failed", "error", actionErr)
		outputBytes = marshalErrorOutput(actionErr)
	} else {
		execLogger.Info("execution completed", "success", actionResult.Success)
		outputBytes = marshalOutput(actionResult)
	}

	execLogger.Info("uploading artifacts to S3", "output_bytes", len(outputBytes))

	return
}

// DispatchAsync creates K8s resources (SA, RBAC, STS Secret, Job) for async execution.
// The single Job pod handles both TA execution and S3 upload using scoped STS credentials.
//
// For kube-api TAs: SA impersonation provides K8s access; STS Secret provides S3 upload only.
// For aws-api TAs: No K8s RBAC needed; STS Secret provides AWS API access + S3 upload.
func (e *Executor) DispatchAsync(ctx context.Context, exec *store.Execution, action actions.Action) error {
	if e.kubeClient == nil || e.stsClient == nil {
		return fmt.Errorf("executor not fully initialized (missing K8s/STS clients)")
	}
	meta := action.Metadata()
	logger := e.logger.With("execution_id", exec.ID, "action", meta.Name, "mode", "async")

	if err := e.ensureNamespace(ctx); err != nil {
		return fmt.Errorf("ensuring namespace: %w", err)
	}

	saName := fmt.Sprintf("zoa-exec-%s", exec.ID)
	logger.Info("creating async execution resources")

	// K8s RBAC only for kube-api TAs; AWS TAs get SA only (no Role/RoleBinding)
	rbacForSA := meta.RBAC
	if meta.Scope == "aws-api" {
		rbacForSA = nil
	}
	if err := e.createResourcesIdempotent(ctx, exec.ID, saName, rbacForSA, exec.Params); err != nil {
		e.cleanupResources(ctx, exec.ID, saName, rbacForSA, exec.Params)
		return fmt.Errorf("creating execution resources: %w", err)
	}

	// Build STS credentials for the Job:
	// - kube-api TAs: assume uploader role (S3 only), session policy restricts to execution prefix
	// - aws-api TAs: assume aws-read/write role (AWS API + S3 upload in same role)
	var stsOut *sts.AssumeRoleOutput
	var err error

	if meta.Scope == "aws-api" {
		roleARN := e.awsReadRoleARN
		if meta.Type == "write" {
			roleARN = e.awsWriteRoleARN
		}
		if roleARN == "" {
			e.cleanupResources(ctx, exec.ID, saName, meta.RBAC, exec.Params)
			return fmt.Errorf("no AWS role configured for async aws-api TA type=%q", meta.Type)
		}
		// No session policy for aws-api TAs: the role's own IAM policy already scopes
		// AWS API access (ec2, eks, etc.) and S3 upload to the artifact bucket.
		// A restrictive session policy would strip the AWS API permissions because
		// session policies act as an intersection with the role policy.
		stsOut, err = e.stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
			RoleArn:         aws.String(roleARN),
			RoleSessionName: aws.String(fmt.Sprintf("zoa-%s", exec.ID[:8])),
			DurationSeconds: aws.Int32(3600),
		})
	} else {
		// kube-api TAs: S3 upload only via uploader role
		sessionPolicy := fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": ["s3:PutObject", "s3:PutObjectTagging"],
					"Resource": "arn:aws:s3:::%s/executions/%s/*"
				},
				{
					"Effect": "Allow",
					"Action": "kms:GenerateDataKey",
					"Resource": "%s"
				}
			]
		}`, e.artifactBucket, exec.ID, e.kmsKeyARN)

		stsOut, err = e.stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
			RoleArn:         aws.String(e.uploaderRoleARN),
			RoleSessionName: aws.String(fmt.Sprintf("zoa-%s", exec.ID[:8])),
			DurationSeconds: aws.Int32(3600),
			Policy:          aws.String(sessionPolicy),
		})
	}

	if err != nil {
		e.cleanupResources(ctx, exec.ID, saName, meta.RBAC, exec.Params)
		return fmt.Errorf("assuming STS role for async execution: %w", err)
	}

	// Create K8s Secret with scoped STS credentials for the Job to use for S3 upload
	credsSecretName := fmt.Sprintf("zoa-creds-%s", exec.ID)
	credsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credsSecretName,
			Namespace: jobNamespace,
			Labels: map[string]string{
				labelKey:                       exec.ID,
				"app.kubernetes.io/managed-by": "zoa",
			},
		},
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte(*stsOut.Credentials.AccessKeyId),
			"AWS_SECRET_ACCESS_KEY": []byte(*stsOut.Credentials.SecretAccessKey),
			"AWS_SESSION_TOKEN":     []byte(*stsOut.Credentials.SessionToken),
			"AWS_DEFAULT_REGION":    []byte(e.region),
		},
	}
	if err := e.withRetry(ctx, "create-creds-secret", func() error {
		_, err := e.kubeClient.CoreV1().Secrets(jobNamespace).Create(ctx, credsSecret, metav1.CreateOptions{})
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}); err != nil {
		e.cleanupResources(ctx, exec.ID, saName, meta.RBAC, exec.Params)
		return fmt.Errorf("creating credentials secret: %w", err)
	}

	timeout := int64(meta.TimeoutSeconds)
	if timeout == 0 {
		timeout = 900
	}
	// Add 300s upload buffer to the Job deadline (STS creds are valid for 3600s)
	jobDeadline := timeout + 300

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("zoa-%s", exec.ID),
			Namespace: jobNamespace,
			Labels: map[string]string{
				labelKey:                       exec.ID,
				"zoa.openshift.io/action":      exec.Action,
				"zoa.openshift.io/target":      exec.TargetCluster,
				"app.kubernetes.io/managed-by": "zoa",
			},
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   &jobDeadline,
			BackoffLimit:            ptr.To(int32(0)),
			TTLSecondsAfterFinished: ptr.To(int32(600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKey:                       exec.ID,
						"app.kubernetes.io/managed-by": "zoa",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "runner",
							Image:   e.jobImage,
							Command: []string{"/usr/local/bin/zoa-runner"},
							Env: []corev1.EnvVar{
								{Name: "EXECUTION_ID", Value: exec.ID},
								{Name: "ACTION", Value: exec.Action},
								{Name: "ARTIFACT_BUCKET", Value: e.artifactBucket},
								{Name: "S3_PREFIX", Value: fmt.Sprintf("executions/%s", exec.ID)},
								{Name: "PARAMS", Value: marshalParamsEnv(exec.Params)},
								{Name: "OPERATOR", Value: exec.Operator},
							},
							// S3 credentials from STS-scoped Secret (mounted as env vars)
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: credsSecretName,
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "output", MountPath: "/output"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:              *parseQuantity("100m"),
									corev1.ResourceMemory:           *parseQuantity("256Mi"),
									corev1.ResourceEphemeralStorage: *parseQuantity("5Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory:           *parseQuantity("1Gi"),
									corev1.ResourceEphemeralStorage: *parseQuantity("10Gi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "output",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	if err := e.withRetry(ctx, "create-job", func() error {
		_, err := e.kubeClient.BatchV1().Jobs(jobNamespace).Create(ctx, job, metav1.CreateOptions{})
		return err
	}); err != nil {
		e.cleanupResources(ctx, exec.ID, saName, rbacForSA, exec.Params)
		return fmt.Errorf("creating job: %w", err)
	}

	logger.Info("async job dispatched",
		"job", job.Name,
		"creds_secret", credsSecretName,
		"job_deadline_s", jobDeadline,
	)
	return nil
}

// CleanupExecution removes all K8s resources associated with an execution (for GC).
func (e *Executor) CleanupExecution(ctx context.Context, executionID string, rbac *actions.RBACConfig, params map[string]string) {
	saName := fmt.Sprintf("zoa-exec-%s", executionID)
	e.cleanupResources(ctx, executionID, saName, rbac, params)
	e.cleanupJob(ctx, executionID)
}

// ArtifactSizes returns the S3 object sizes and output format for an execution's artifacts.
// Checks output.json first; falls back to output.tar.gz. Returns 0/empty for missing objects.
func (e *Executor) ArtifactSizes(ctx context.Context, executionID string) (outputBytes, logBytes int64, outputFormat string) {
	jsonKey := fmt.Sprintf("executions/%s/output.json", executionID)
	if out, err := e.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &e.artifactBucket,
		Key:    &jsonKey,
	}); err == nil && out.ContentLength != nil {
		outputBytes = *out.ContentLength
		outputFormat = "json"
	} else {
		tarKey := fmt.Sprintf("executions/%s/output.tar.gz", executionID)
		if out, err := e.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: &e.artifactBucket,
			Key:    &tarKey,
		}); err == nil && out.ContentLength != nil {
			outputBytes = *out.ContentLength
			outputFormat = "tar.gz"
		}
	}

	logKey := fmt.Sprintf("executions/%s/execution.log", executionID)
	if out, err := e.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &e.artifactBucket,
		Key:    &logKey,
	}); err == nil && out.ContentLength != nil {
		logBytes = *out.ContentLength
	}
	return
}

func (e *Executor) cleanupJob(ctx context.Context, executionID string) {
	jobName := fmt.Sprintf("zoa-%s", executionID)
	propagation := metav1.DeletePropagationBackground
	err := e.kubeClient.BatchV1().Jobs(jobNamespace).Delete(ctx, jobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil {
		e.logger.Debug("job cleanup (may not exist)", "job", jobName, "error", err)
	}
}

func (e *Executor) buildAWSParams(ctx context.Context, executionID string, params map[string]string, meta actions.ActionMetadata, logger *slog.Logger) (*actions.ExecutionParams, error) {
	if e.stsClient == nil {
		return nil, fmt.Errorf("STS client not configured (AWS_READ_ROLE_ARN/AWS_WRITE_ROLE_ARN env vars required for aws-api TAs)")
	}

	roleARN := e.awsReadRoleARN
	if meta.Type == "write" {
		roleARN = e.awsWriteRoleARN
	}

	if roleARN == "" {
		return nil, fmt.Errorf("no AWS role configured for scope=%q type=%q (set AWS_READ_ROLE_ARN/AWS_WRITE_ROLE_ARN)", meta.Scope, meta.Type)
	}

	assumeOut, err := e.stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(fmt.Sprintf("zoa-%s", executionID)),
		DurationSeconds: aws.Int32(900),
	})
	if err != nil {
		return nil, fmt.Errorf("assuming AWS role %s: %w", roleARN, err)
	}

	scopedCfg := aws.Config{
		Region: e.region,
		Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     *assumeOut.Credentials.AccessKeyId,
				SecretAccessKey: *assumeOut.Credentials.SecretAccessKey,
				SessionToken:    *assumeOut.Credentials.SessionToken,
				Source:          "zoa-sts-assume-role",
			}, nil
		}),
	}

	logger.Info("assumed AWS role for TA execution", "role", roleARN, "type", meta.Type)

	return &actions.ExecutionParams{
		Params:      params,
		ExecutionID: executionID,
		AWSConfig:   &scopedCfg,
		Logger:      logger,
	}, nil
}

func (e *Executor) buildKubeParams(ctx context.Context, executionID string, params map[string]string, meta actions.ActionMetadata, logger *slog.Logger) (*actions.ExecutionParams, error) {
	if err := e.ensureNamespace(ctx); err != nil {
		return nil, fmt.Errorf("ensuring namespace: %w", err)
	}

	saName := fmt.Sprintf("zoa-exec-%s", executionID)

	e.logger.Info("creating execution resources", "execution_id", executionID)
	if err := e.createResourcesIdempotent(ctx, executionID, saName, meta.RBAC, params); err != nil {
		e.cleanupResources(ctx, executionID, saName, meta.RBAC, params)
		return nil, fmt.Errorf("creating execution resources: %w", err)
	}

	execClient, dynClient, execConfig, err := e.clientForServiceAccount(ctx, saName)
	if err != nil {
		e.cleanupResources(ctx, executionID, saName, meta.RBAC, params)
		return nil, fmt.Errorf("creating scoped client: %w", err)
	}

	return &actions.ExecutionParams{
		Params:        params,
		ExecutionID:   executionID,
		TargetCluster: "",
		KubeClient:    execClient,
		DynamicClient: dynClient,
		RESTConfig:    execConfig,
		Logger:        logger,
	}, nil
}

func (e *Executor) uploadSyncArtifacts(ctx context.Context, executionID string, output, logs []byte, logger *slog.Logger) (int64, int64) {
	var outputLen, logLen int64

	if len(output) > 0 {
		outputKey := fmt.Sprintf("executions/%s/output.json", executionID)
		if _, err := e.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      &e.artifactBucket,
			Key:         &outputKey,
			Body:        bytes.NewReader(output),
			ContentType: aws.String("application/json"),
		}); err != nil {
			logger.Error("failed to upload output.json", "error", err)
		} else {
			outputLen = int64(len(output))
		}
	}

	if len(logs) > 0 {
		logKey := fmt.Sprintf("executions/%s/execution.log", executionID)
		if _, err := e.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      &e.artifactBucket,
			Key:         &logKey,
			Body:        bytes.NewReader(logs),
			ContentType: aws.String("text/plain"),
		}); err != nil {
			logger.Error("failed to upload execution.log", "error", err)
		} else {
			logLen = int64(len(logs))
		}
	}

	logger.Info("artifacts uploaded to S3",
		"execution_id", executionID,
		"output_bytes", outputLen,
		"log_bytes", logLen,
	)

	return outputLen, logLen
}

// MarshalActionOutput serializes only the Output field of an ActionResult.
// This is the canonical format for output.json in S3 — both sync and async
// paths MUST use this function to ensure consistent CLI rendering.
func MarshalActionOutput(result *actions.ActionResult) []byte {
	if result == nil || result.Output == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(result.Output)
	if err != nil {
		return []byte(fmt.Sprintf(`{"error":"failed to marshal output: %s"}`, err.Error()))
	}
	return data
}

func marshalOutput(result *actions.ActionResult) []byte {
	return MarshalActionOutput(result)
}

func marshalErrorOutput(err error) []byte {
	data, _ := json.Marshal(map[string]string{"error": err.Error()})
	return data
}

func marshalParamsEnv(params map[string]string) string {
	if len(params) == 0 {
		return "{}"
	}
	data, _ := json.Marshal(params)
	return string(data)
}

func parseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}
