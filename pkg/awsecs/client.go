// Package awsecs provides a concrete AWS ECS client that satisfies
// the ECSAPI interfaces used by the access handler and session reaper.
package awsecs

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/api"
)

// Client wraps the AWS ECS SDK and implements api.ECSAPI.
type Client struct {
	ecs *ecs.Client
}

// New creates a new ECS client from an AWS config.
func New(cfg aws.Config) *Client {
	return &Client{ecs: ecs.NewFromConfig(cfg)}
}

// RunTask implements api.ECSAPI.RunTask — starts an ECS Fargate task
// with the given configuration and returns the task ARN and task ID.
func (c *Client) RunTask(ctx context.Context, input *api.RunTaskInput) (*api.RunTaskOutput, error) {
	envPairs := make([]ecstypes.KeyValuePair, 0, len(input.Environment))
	for k, v := range input.Environment {
		envPairs = append(envPairs, ecstypes.KeyValuePair{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	tags := make([]ecstypes.Tag, 0, len(input.Tags))
	for k, v := range input.Tags {
		tags = append(tags, ecstypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	out, err := c.ecs.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:              aws.String(input.Cluster),
		TaskDefinition:       aws.String(input.TaskDefinition),
		LaunchType:           ecstypes.LaunchTypeFargate,
		Count:                aws.Int32(1),
		EnableExecuteCommand: true,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        input.Subnets,
				SecurityGroups: []string{input.SecurityGroup},
				AssignPublicIp: ecstypes.AssignPublicIpDisabled,
			},
		},
		Overrides: &ecstypes.TaskOverride{
			ContainerOverrides: []ecstypes.ContainerOverride{
				{
					Name:        aws.String("zoa-boundary"),
					Environment: envPairs,
				},
			},
		},
		Tags: tags,
	})
	if err != nil {
		return nil, fmt.Errorf("ecs RunTask: %w", err)
	}

	if len(out.Tasks) == 0 {
		if len(out.Failures) > 0 {
			return nil, fmt.Errorf("ecs RunTask failed: %s — %s",
				aws.ToString(out.Failures[0].Reason),
				aws.ToString(out.Failures[0].Detail))
		}
		return nil, fmt.Errorf("ecs RunTask returned no tasks and no failures")
	}

	taskArn := aws.ToString(out.Tasks[0].TaskArn)
	taskID := extractTaskID(taskArn)

	return &api.RunTaskOutput{
		TaskArn: taskArn,
		TaskID:  taskID,
	}, nil
}

// StopTask implements api.ECSAPI.StopTask — stops an ECS task.
func (c *Client) StopTask(ctx context.Context, input *api.StopTaskInput) error {
	_, err := c.ecs.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(input.Cluster),
		Task:    aws.String(input.TaskArn),
		Reason:  aws.String(input.Reason),
	})
	if err != nil {
		return fmt.Errorf("ecs StopTask %s: %w", input.TaskArn, err)
	}
	return nil
}

// ReaperAdapter wraps Client to satisfy the scheduler.ECSAPI interface
// (which uses positional args instead of struct input for StopTask).
type ReaperAdapter struct {
	client *Client
}

// NewReaperAdapter creates a scheduler.ECSAPI-compatible wrapper.
func NewReaperAdapter(c *Client) *ReaperAdapter {
	return &ReaperAdapter{client: c}
}

// StopTask implements scheduler.ECSAPI.StopTask with positional args.
func (a *ReaperAdapter) StopTask(ctx context.Context, cluster, taskArn, reason string) error {
	return a.client.StopTask(ctx, &api.StopTaskInput{
		Cluster: cluster,
		TaskArn: taskArn,
		Reason:  reason,
	})
}

// extractTaskID extracts the task UUID from an ECS task ARN.
// ARN format: arn:aws:ecs:region:account:task/cluster-name/task-uuid
func extractTaskID(taskArn string) string {
	// Find the last "/" and return everything after it.
	for i := len(taskArn) - 1; i >= 0; i-- {
		if taskArn[i] == '/' {
			return taskArn[i+1:]
		}
	}
	return taskArn
}
