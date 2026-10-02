// Package execcreds vends ECS Exec credentials scoped to a single boundary task.
package execcreds

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
)

const defaultDuration = time.Hour

// APICredentials is returned to the ZOA CLI on session join (JSON field exec_credentials).
type APICredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	Expiration      string `json:"expiration"`
}

// AssumeRoleAPI abstracts STS AssumeRole for tests.
type AssumeRoleAPI interface {
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

// Vendor mints per-task ECS Exec credentials via sts:AssumeRole session policies.
type Vendor struct {
	client   AssumeRoleAPI
	duration time.Duration
}

// NewVendor creates a Vendor backed by the given STS client.
func NewVendor(client AssumeRoleAPI, duration time.Duration) *Vendor {
	if duration <= 0 {
		duration = defaultDuration
	}
	return &Vendor{
		client:   client,
		duration: duration,
	}
}

// VendForTask returns temporary credentials that can ecs:ExecuteCommand only on the given task.
func (v *Vendor) VendForTask(ctx context.Context, execRoleARN, username, clusterARN, taskARN string) (*APICredentials, error) {
	if v == nil || v.client == nil {
		return nil, fmt.Errorf("exec credential vendor not configured")
	}
	if execRoleARN == "" {
		return nil, fmt.Errorf("exec scoped role ARN not configured")
	}
	if username == "" || clusterARN == "" || taskARN == "" {
		return nil, fmt.Errorf("username, cluster ARN, and task ARN are required")
	}

	policy, err := sessionPolicy(clusterARN, taskARN)
	if err != nil {
		return nil, err
	}

	durationSeconds := int32(v.duration.Seconds())
	if durationSeconds < 900 {
		durationSeconds = 900
	}
	if durationSeconds > 43200 {
		durationSeconds = 43200
	}

	out, err := v.client.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(execRoleARN),
		RoleSessionName: aws.String(sanitizeSessionName(username)),
		DurationSeconds: aws.Int32(durationSeconds),
		Policy:          aws.String(policy),
	})
	if err != nil {
		return nil, fmt.Errorf("assume exec-scoped role: %w", err)
	}
	if out.Credentials == nil {
		return nil, fmt.Errorf("assume exec-scoped role: empty credentials")
	}

	return apiCredentialsFromSTS(out.Credentials), nil
}

func apiCredentialsFromSTS(c *types.Credentials) *APICredentials {
	exp := ""
	if c.Expiration != nil {
		exp = c.Expiration.UTC().Format(time.RFC3339)
	}
	return &APICredentials{
		AccessKeyID:     aws.ToString(c.AccessKeyId),
		SecretAccessKey: aws.ToString(c.SecretAccessKey),
		SessionToken:    aws.ToString(c.SessionToken),
		Expiration:      exp,
	}
}

func sessionPolicy(clusterARN, taskARN string) (string, error) {
	doc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Sid":    "ExecuteCommandOnSessionTask",
				"Effect": "Allow",
				"Action": []string{"ecs:ExecuteCommand"},
				"Resource": []string{
					clusterARN,
					taskARN,
				},
			},
			{
				"Sid":      "DescribeSessionTask",
				"Effect":   "Allow",
				"Action":   []string{"ecs:DescribeTasks"},
				"Resource": taskARN,
			},
			{
				"Sid":    "SSMMessagesForExec",
				"Effect": "Allow",
				"Action": []string{
					"ssmmessages:CreateControlChannel",
					"ssmmessages:CreateDataChannel",
					"ssmmessages:OpenControlChannel",
					"ssmmessages:OpenDataChannel",
				},
				"Resource": "*",
			},
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal session policy: %w", err)
	}
	return string(b), nil
}

func sanitizeSessionName(username string) string {
	const maxLen = 64
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '=', r == ',', r == '@', r == '-':
			return r
		default:
			return '-'
		}
	}, username)
	if s == "" {
		s = "zoa-exec"
	}
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}
