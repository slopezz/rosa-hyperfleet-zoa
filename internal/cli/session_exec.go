package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/ecsexec"
)

// execAWSProfileEnv is the optional profile for ECS Exec (regional account).
// Invoker role credentials cannot call ecs:ExecuteCommand.
const execAWSProfileEnv = "ZOA_EXEC_AWS_PROFILE"

func loadExecAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if profile := os.Getenv(execAWSProfileEnv); profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return cfg, fmt.Errorf("loading AWS config for ECS Exec: %w", err)
	}
	return cfg, nil
}

func runSessionECSExec(ctx context.Context, region string, join *client.SessionJoinResponse) error {
	if join.TaskArn == "" {
		return fmt.Errorf("session has no task ARN")
	}
	cfg, err := loadExecAWSConfig(ctx, region)
	if err != nil {
		return err
	}
	return ecsexec.ConnectInteractive(ctx, cfg, ecsexec.JoinParams{
		Region:        region,
		Cluster:       join.ClusterArn,
		TaskARN:       join.TaskArn,
		ContainerName: join.ContainerName,
	})
}

func sessionJoinRegion(opts *GlobalOptions, join *client.SessionJoinResponse) string {
	if join.Region != "" {
		return join.Region
	}
	return opts.Region
}
