// Binary zoa-lambda is the Lambda entry point. It performs dependency injection
// and starts a native Lambda handler for both API and Worker modes.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/eksauth"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/api"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/handler"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/lambdahttp"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/scheduler"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

func main() {
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	actions.SetDeploymentTarget(cfg.DeploymentTarget)

	logger = logger.With("target", cfg.TargetCluster, "deployment_target", cfg.DeploymentTarget, "region", cfg.Region)
	logger.Info("starting zoa-lambda",
		"handler_mode", cfg.HandlerMode,
		"reconciler_deadline_s", cfg.ReconcilerDeadlineSeconds,
		"execution_deadline_s", cfg.ExecutionDeadlineSeconds,
		"max_batch_per_tick", cfg.MaxBatchPerTick,
	)

	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer initCancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(initCtx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		logger.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}

	// For cross-account data access (MC→RC), assume the data-access role to get
	// credentials that target the RC account's DynamoDB tables and S3 bucket.
	dataStoreCfg := awsCfg
	if cfg.DataStoreRoleARN != "" {
		dataStoreCfg, err = awsconfig.LoadDefaultConfig(initCtx,
			awsconfig.WithRegion(cfg.Region),
			awsconfig.WithCredentialsProvider(
				stscreds.NewAssumeRoleProvider(
					sts.NewFromConfig(awsCfg),
					cfg.DataStoreRoleARN,
				),
			),
		)
		if err != nil {
			logger.Error("failed to assume data-access role", "role", cfg.DataStoreRoleARN, "error", err)
			os.Exit(1)
		}
		logger.Info("cross-account data access configured", "role", cfg.DataStoreRoleARN)
	}

	dynamoClient := dynamodb.NewFromConfig(dataStoreCfg)
	s3Client := s3.NewFromConfig(dataStoreCfg)
	execStore := store.NewExecutionStore(dynamoClient, cfg.ExecutionsTable, cfg.DynamoDBTTLDays)

	var restCfg *rest.Config
	var kubeClient kubernetes.Interface

	restCfg, err = eksauth.NewRESTConfig(cfg.EKSClusterEndpoint, cfg.EKSClusterCA, cfg.EKSClusterName, awsCfg)
	if err != nil {
		logger.Error("failed to build EKS REST config", "error", err,
			"endpoint", cfg.EKSClusterEndpoint)
		os.Exit(1)
	}

	kubeClient, err = kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error("failed to create kubernetes client", "error", err)
		os.Exit(1)
	}

	if err := handler.CheckStartupHealth(initCtx, handler.HealthDeps{
		DynamoClient: dynamoClient,
		S3Client:     s3Client,
		KubeClient:   kubeClient,
		TableName:    cfg.ExecutionsTable,
		BucketName:   cfg.ArtifactBucket,
		Mode:         cfg.HandlerMode,
	}, logger); err != nil {
		logger.Error("startup health check failed — exiting", "error", err)
		os.Exit(1)
	}

	exec := executor.New(kubeClient, restCfg, s3Client, &awsCfg, executor.ExecutorConfig{
		ArtifactBucket:   cfg.ArtifactBucket,
		UploaderRoleARN:  cfg.UploaderRoleARN,
		AWSReadRoleARN:   cfg.AWSReadRoleARN,
		AWSWriteRoleARN:  cfg.AWSWriteRoleARN,
		KMSKeyARN:        cfg.KMSKeyARN,
		Region:           cfg.Region,
		JobImage:         cfg.JobImage,
		DeploymentTarget: cfg.DeploymentTarget,
		TargetCluster:    cfg.TargetCluster,
	}, logger)

	switch cfg.HandlerMode {
	case "api":
		// API mode: native Lambda handler with Function URL streaming.
		// Converts Function URL events to http.Request, serves via the HTTP handler,
		// and returns a streaming response (up to 200MB).
		auditStore := store.NewAuditStore(dynamoClient, cfg.AuditTable, cfg.DynamoDBTTLDays)
		apiHandler := api.New(cfg, execStore, auditStore, exec, s3Client, logger)
		streamingHandler := lambdahttp.NewStreamingHandler(apiHandler)

		logger.Info("API mode: native Lambda Function URL streaming handler")
		lambda.Start(streamingHandler.Handle)

	case "worker":
		// Worker mode: native Lambda handler. Receives EventBridge schedules
		// and self-invocation events (no HTTP involved).
		lambdaClient := awslambda.NewFromConfig(awsCfg)
		reconciler := scheduler.NewReconciler(execStore, kubeClient, lambdaClient, exec, cfg, logger)

		h := handler.New(handler.Deps{
			Cfg:        cfg,
			Reconciler: reconciler,
			Executor:   exec,
			ExecStore:  execStore,
			Logger:     logger,
		})
		lambda.Start(h.HandleEvent)

	default:
		logger.Error("unknown handler mode — cannot start Lambda", "mode", cfg.HandlerMode)
		os.Exit(1)
	}
}
