package cli

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/ecsexec"
)

func runSessionECSExec(ctx context.Context, _ string, region string, join *client.SessionJoinResponse) error {
	if join.TaskArn == "" {
		return fmt.Errorf("session has no task ARN")
	}
	if join.ExecCommand == "" {
		return fmt.Errorf("join response missing exec_command")
	}
	if join.ExecCredentials == nil || join.ExecCredentials.AccessKeyID == "" {
		return fmt.Errorf("join response missing exec_credentials")
	}

	cfg, err := awsConfigFromJoin(ctx, region, join)
	if err != nil {
		return err
	}
	return ecsexec.ConnectInteractive(ctx, cfg, ecsexec.JoinParams{
		Region:        region,
		Cluster:       join.ClusterArn,
		TaskARN:       join.TaskArn,
		ContainerName: join.ContainerName,
		Command:       join.ExecCommand,
	})
}

func awsConfigFromJoin(ctx context.Context, region string, join *client.SessionJoinResponse) (aws.Config, error) {
	reg := region
	if join.Region != "" {
		reg = join.Region
	}
	if reg == "" {
		return aws.Config{}, fmt.Errorf("session join response missing region")
	}
	c := join.ExecCredentials
	return awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(reg),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			c.AccessKeyID,
			c.SecretAccessKey,
			c.SessionToken,
		)),
	)
}

func sessionJoinRegion(opts *GlobalOptions, join *client.SessionJoinResponse) string {
	if join.Region != "" {
		return join.Region
	}
	if opts != nil && opts.Region != "" {
		return opts.Region
	}
	return ""
}
