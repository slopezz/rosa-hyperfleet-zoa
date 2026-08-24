// Binary zoa-runner executes a Trusted Action inside a K8s Job pod.
// It runs the TA, captures output and logs to /output/, then uploads
// artifacts to S3 using STS-scoped credentials from environment variables.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
)

const outputDir = "/output"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	executionID := os.Getenv("EXECUTION_ID")
	actionName := os.Getenv("ACTION")
	bucket := os.Getenv("ARTIFACT_BUCKET")
	s3Prefix := os.Getenv("S3_PREFIX")
	region := os.Getenv("AWS_DEFAULT_REGION")

	if executionID == "" || actionName == "" || bucket == "" || s3Prefix == "" {
		logger.Error("missing required environment variables",
			"EXECUTION_ID", executionID, "ACTION", actionName,
			"ARTIFACT_BUCKET", bucket, "S3_PREFIX", s3Prefix)
		os.Exit(1)
	}

	logger = logger.With("execution_id", executionID, "action", actionName)
	logger.Info("zoa-runner starting")

	action, ok := actions.Get(actionName)
	if !ok {
		logger.Error("action not found in registry")
		writeExitStatus(2)
		os.Exit(2)
	}

	params := parseParams(os.Getenv("PARAMS"))

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		logger.Error("failed to create output directory", "error", err)
		writeExitStatus(1)
		os.Exit(1)
	}

	logFile, err := os.Create(filepath.Join(outputDir, "execution.log"))
	if err != nil {
		logger.Error("failed to create execution.log", "error", err)
		writeExitStatus(1)
		os.Exit(1)
	}
	defer logFile.Close()

	// Tee logs to both stdout and execution.log
	logWriter := io.MultiWriter(os.Stdout, logFile)
	execLogger := slog.New(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelInfo}))

	meta := action.Metadata()
	timeout := time.Duration(meta.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	execParams := &actions.ExecutionParams{
		Params:      params,
		ExecutionID: executionID,
		Logger:      execLogger,
	}

	// Build Kubernetes clients from in-cluster config (pod has projected SA token)
	if restCfg, err := rest.InClusterConfig(); err == nil {
		kubeClient, kubeErr := kubernetes.NewForConfig(restCfg)
		if kubeErr != nil {
			execLogger.Error("failed to create kube client", "error", kubeErr)
		} else {
			execParams.KubeClient = kubeClient
		}

		dynClient, dynErr := dynamic.NewForConfig(restCfg)
		if dynErr != nil {
			execLogger.Error("failed to create dynamic client", "error", dynErr)
		} else {
			execParams.DynamicClient = dynClient
		}

		execParams.RESTConfig = restCfg
	} else {
		execLogger.Warn("in-cluster config not available", "error", err)
	}

	// Build AWS config from env credentials (injected from STS Secret for aws-api TAs)
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		awsCfg := aws.Config{
			Region: region,
			Credentials: credentials.NewStaticCredentialsProvider(
				os.Getenv("AWS_ACCESS_KEY_ID"),
				os.Getenv("AWS_SECRET_ACCESS_KEY"),
				os.Getenv("AWS_SESSION_TOKEN"),
			),
		}
		execParams.AWSConfig = &awsCfg
	}

	execLogger.Info("executing action", "timeout_seconds", meta.TimeoutSeconds)

	result, execErr := action.Execute(ctx, execParams)
	if execErr != nil {
		execLogger.Error("action execution failed", "error", execErr)
	}

	exitCode := 0
	if result == nil {
		result = &actions.ActionResult{Success: false, Summary: "execution failed: " + execErr.Error()}
		exitCode = 1
	} else if !result.Success {
		exitCode = 1
	}

	// Write output.json only when the action produces structured JSON output.
	// Actions that produce binary artifacts (e.g. must_gather → output.tar.gz)
	// set Output=nil so ArtifactSizes() detects the tar.gz as primary artifact.
	if result.Output != nil {
		outputData := executor.MarshalActionOutput(result)
		if err := os.WriteFile(filepath.Join(outputDir, "output.json"), outputData, 0o644); err != nil {
			execLogger.Error("failed to write output.json", "error", err)
		}
	} else {
		execLogger.Info("skipping output.json (action produced binary artifact)")
	}

	// Upload artifacts to S3
	execLogger.Info("uploading artifacts to S3", "bucket", bucket, "prefix", s3Prefix)
	if err := uploadArtifacts(bucket, s3Prefix, region, logger); err != nil {
		execLogger.Error("S3 upload failed", "error", err)
		os.Exit(1)
	}

	writeExitStatus(exitCode)
	logger.Info("zoa-runner completed", "exit_code", exitCode)
	os.Exit(exitCode)
}

func uploadArtifacts(bucket, prefix, region string, logger *slog.Logger) error {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("AWS credentials not found in environment (expected from STS Secret)")
	}

	creds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)
	s3Client := s3.New(s3.Options{
		Region:      region,
		Credentials: creds,
	})

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("reading output directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(outputDir, entry.Name())
		key := prefix + "/" + entry.Name()

		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("failed to read file for upload", "file", filePath, "error", err)
			continue
		}

		contentType := "application/octet-stream"
		if strings.HasSuffix(entry.Name(), ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(entry.Name(), ".log") {
			contentType = "text/plain"
		} else if strings.HasSuffix(entry.Name(), ".tar.gz") || strings.HasSuffix(entry.Name(), ".tgz") {
			contentType = "application/gzip"
		}

		_, err = s3Client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(contentType),
		})
		if err != nil {
			return fmt.Errorf("uploading %s: %w", key, err)
		}
		logger.Info("uploaded", "key", key, "size_bytes", len(data))
	}
	return nil
}

func parseParams(raw string) map[string]string {
	params := make(map[string]string)
	if raw == "" {
		return params
	}
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return params
	}
	return params
}

func writeExitStatus(code int) {
	_ = os.WriteFile(filepath.Join(outputDir, "exit_code"), []byte(fmt.Sprintf("%d", code)), 0o644)
}
