package ecsexec

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// Client performs ECS Exec against a single cluster.
type Client struct {
	client  *ecs.Client
	cluster string
}

// NewClient creates an ECS client. cluster may be a cluster name or ARN.
func NewClient(cfg aws.Config, cluster string) *Client {
	clusterName := ClusterNameFromARN(cluster)
	return &Client{
		client:  ecs.NewFromConfig(cfg),
		cluster: clusterName,
	}
}

// Session holds ExecuteCommand output for the session-manager-plugin.
type Session struct {
	SessionID  string
	Target     string
	RawSession json.RawMessage
}

// WaitForRunning waits until the task reaches RUNNING.
func (c *Client) WaitForRunning(ctx context.Context, taskID string) error {
	waiter := ecs.NewTasksRunningWaiter(c.client)
	return waiter.Wait(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(c.cluster),
		Tasks:   []string{taskID},
	}, 10*time.Minute)
}

// WaitForExecAgent polls until the container ExecuteCommand agent is RUNNING.
func (c *Client) WaitForExecAgent(ctx context.Context, taskID, container string, maxWait time.Duration) error {
	const pollInterval = 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()
	for {
		ready, err := c.isExecAgentRunning(ctx, taskID, container)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (c *Client) isExecAgentRunning(ctx context.Context, taskID, container string) (bool, error) {
	out, err := c.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(c.cluster),
		Tasks:   []string{taskID},
	})
	if err != nil {
		return false, fmt.Errorf("DescribeTasks: %w", err)
	}
	if len(out.Tasks) == 0 {
		return false, fmt.Errorf("task %s not found in cluster %s", taskID, c.cluster)
	}
	for _, cont := range out.Tasks[0].Containers {
		if aws.ToString(cont.Name) != container {
			continue
		}
		for _, agent := range cont.ManagedAgents {
			if agent.Name == types.ManagedAgentNameExecuteCommandAgent {
				return aws.ToString(agent.LastStatus) == "RUNNING", nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("container %q not found in task %s", container, taskID)
}

// ExecuteCommand starts an interactive ECS Exec session.
func (c *Client) ExecuteCommand(ctx context.Context, taskID, container, command string) (*Session, error) {
	out, err := c.client.ExecuteCommand(ctx, &ecs.ExecuteCommandInput{
		Cluster:     aws.String(c.cluster),
		Task:        aws.String(taskID),
		Container:   aws.String(container),
		Command:     aws.String(command),
		Interactive: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ExecuteCommand: %w", err)
	}
	if out.Session == nil {
		return nil, fmt.Errorf("ExecuteCommand returned nil session")
	}

	var target string
	descOut, descErr := c.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(c.cluster),
		Tasks:   []string{taskID},
	})
	if descErr == nil && len(descOut.Tasks) > 0 {
		for _, cont := range descOut.Tasks[0].Containers {
			if aws.ToString(cont.Name) == container && cont.RuntimeId != nil {
				target = fmt.Sprintf("ecs:%s_%s_%s", c.cluster, taskID, aws.ToString(cont.RuntimeId))
				break
			}
		}
	}

	sessionPayload := map[string]string{
		"sessionId":  aws.ToString(out.Session.SessionId),
		"streamUrl":  aws.ToString(out.Session.StreamUrl),
		"tokenValue": aws.ToString(out.Session.TokenValue),
	}
	rawSession, err := json.Marshal(sessionPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal session: %w", err)
	}

	return &Session{
		SessionID:  aws.ToString(out.Session.SessionId),
		Target:     target,
		RawSession: rawSession,
	}, nil
}
