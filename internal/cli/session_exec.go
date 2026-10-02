package cli

import (
	"context"
	"fmt"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/accessclient"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/ecsexec"
)

func runSessionECSExec(ctx context.Context, deployment, region string, join *client.SessionJoinResponse) error {
	if join.TaskArn == "" {
		return fmt.Errorf("session has no task ARN")
	}
	if join.ExecCommand == "" {
		return fmt.Errorf("Access API did not return exec_command (check ZOA_ECS_EXEC_COMMAND on Access Lambda)")
	}
	cfg, err := accessclient.ExecAWSConfig(ctx, deployment)
	if err != nil {
		return err
	}
	if region == "" {
		region = cfg.Region
	}
	return ecsexec.ConnectInteractive(ctx, cfg, ecsexec.JoinParams{
		Region:        region,
		Cluster:       join.ClusterArn,
		TaskARN:       join.TaskArn,
		ContainerName: join.ContainerName,
		Command:       join.ExecCommand,
	})
}

func sessionJoinRegion(opts *GlobalOptions, join *client.SessionJoinResponse) string {
	if join.Region != "" {
		return join.Region
	}
	return opts.Region
}
