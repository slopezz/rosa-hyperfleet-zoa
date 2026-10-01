package ecsexec

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// JoinParams identifies a boundary task to connect to.
type JoinParams struct {
	Region        string
	Cluster       string // cluster name or ARN
	TaskARN       string
	ContainerName string
	Command       string
	NoWait        bool
}

const defaultExecCommand = "/bin/bash"

// ConnectInteractive waits for the task, starts ECS Exec, and execs session-manager-plugin.
func ConnectInteractive(ctx context.Context, cfg aws.Config, p JoinParams) error {
	if p.ContainerName == "" {
		p.ContainerName = "zoa-boundary"
	}
	if p.Command == "" {
		p.Command = defaultExecCommand
	}

	clusterName, taskID, err := ClusterAndTaskFromTaskARN(p.TaskARN)
	if err != nil {
		return err
	}
	if p.Cluster != "" {
		clusterName = ClusterNameFromARN(p.Cluster)
	}

	ecsClient := NewClient(cfg, clusterName)

	var session *Session

	if !p.NoWait {
		if err := runWithSpinner(ctx, "boundary task starting", func(ctx context.Context) error {
			return ecsClient.WaitForRunning(ctx, taskID)
		}); err != nil {
			return fmt.Errorf("task not running: %w", err)
		}
		if err := runWithSpinner(ctx, "ECS Exec agent", func(ctx context.Context) error {
			return ecsClient.WaitForExecAgent(ctx, taskID, p.ContainerName, 30*time.Second)
		}); err != nil {
			return fmt.Errorf("exec agent not ready: %w", err)
		}
	}

	if err := runWithSpinner(ctx, "opening ECS Exec", func(ctx context.Context) error {
		var execErr error
		session, execErr = ecsClient.ExecuteCommand(ctx, taskID, p.ContainerName, p.Command)
		return execErr
	}); err != nil {
		return err
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("retrieve AWS credentials for exec: %w", err)
	}

	region := p.Region
	if region == "" {
		region = cfg.Region
	}
	return StartSessionManagerPlugin(region, session, creds)
}
