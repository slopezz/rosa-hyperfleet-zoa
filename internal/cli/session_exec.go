package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/ecsexec"
)

func runSessionECSExec(ctx context.Context, _ string, region string, zoaSessionID string, access APIClient, join *client.SessionJoinResponse) error {
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
		OnExecSession: func(execSessionID string) error {
			if access == nil {
				fmt.Fprintln(os.Stderr, "Warning: exec session not registered (access client not configured); idle reaper still uses AWS exec logs")
				return nil
			}
			if err := access.SessionExecAttached(ctx, zoaSessionID, execSessionID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not register exec session with Access API (%v); continuing to shell (idle reaper still uses AWS exec logs)\n", err)
			}
			return nil
		},
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
