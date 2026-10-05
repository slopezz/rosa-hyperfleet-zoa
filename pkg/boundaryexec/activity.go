package boundaryexec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

const ecsExecDocument = "AmazonECS-ExecuteInteractiveCommand"

// ActivityChecker resolves ECS Exec terminal activity via SSM session history and CloudWatch Logs.
type ActivityChecker struct {
	ssm  *ssm.Client
	logs *cloudwatchlogs.Client
}

func NewActivityChecker(cfg aws.Config) *ActivityChecker {
	return &ActivityChecker{
		ssm:  ssm.NewFromConfig(cfg),
		logs: cloudwatchlogs.NewFromConfig(cfg),
	}
}

// LastTerminalActivity returns the latest known exec log event time for taskID on targetCluster.
// Exec sessions are resolved from Dynamo hints and SSM session history for the task.
// found is false when no exec sessions exist at AWS; the reaper may still terminate using session age.
func (c *ActivityChecker) LastTerminalActivity(ctx context.Context, targetCluster, taskID string, dynamoExecIDs []string) (time.Time, bool, error) {
	if c == nil || taskID == "" || targetCluster == "" {
		return time.Time{}, false, nil
	}

	streamNames := make(map[string]struct{})
	for _, id := range dynamoExecIDs {
		streamNames[LogStreamName(id)] = struct{}{}
	}

	ssmIDs, err := c.listExecSessionIDs(ctx, taskID)
	if err != nil {
		return time.Time{}, false, err
	}
	for _, id := range ssmIDs {
		streamNames[LogStreamName(id)] = struct{}{}
	}
	if len(streamNames) == 0 {
		return time.Time{}, false, nil
	}

	logGroup := ExecLogGroup(targetCluster)
	var latest time.Time
	for name := range streamNames {
		ts, err := c.lastStreamEventTime(ctx, logGroup, name)
		if err != nil {
			return time.Time{}, false, err
		}
		if ts.After(latest) {
			latest = ts
		}
	}
	if latest.IsZero() {
		return time.Time{}, false, nil
	}
	return latest, true, nil
}

func (c *ActivityChecker) listExecSessionIDs(ctx context.Context, taskID string) ([]string, error) {
	var ids []string
	var nextToken *string
	for pages := 0; pages < 20; pages++ {
		out, err := c.ssm.DescribeSessions(ctx, &ssm.DescribeSessionsInput{
			State:      ssmtypes.SessionStateHistory,
			MaxResults: aws.Int32(50),
			NextToken:  nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("ssm DescribeSessions: %w", err)
		}
		for _, sess := range out.Sessions {
			if aws.ToString(sess.DocumentName) != ecsExecDocument {
				continue
			}
			target := aws.ToString(sess.Target)
			if !strings.Contains(target, taskID) || !strings.Contains(target, "zoa-boundary") {
				continue
			}
			sid := aws.ToString(sess.SessionId)
			if sid != "" {
				ids = append(ids, sid)
			}
		}
		nextToken = out.NextToken
		if nextToken == nil {
			break
		}
	}
	return ids, nil
}

func (c *ActivityChecker) lastStreamEventTime(ctx context.Context, logGroup, streamName string) (time.Time, error) {
	out, err := c.logs.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(logGroup),
		LogStreamNamePrefix: aws.String(streamName),
		Limit:               aws.Int32(1),
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("describe log stream %q: %w", streamName, err)
	}
	if len(out.LogStreams) == 0 {
		return time.Time{}, nil
	}
	ls := out.LogStreams[0]
	if ls.LastEventTimestamp != nil {
		return time.UnixMilli(*ls.LastEventTimestamp), nil
	}
	if ls.LastIngestionTime != nil {
		return time.UnixMilli(*ls.LastIngestionTime), nil
	}
	return time.Time{}, nil
}
