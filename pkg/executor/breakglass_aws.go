package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// BreakglassAWSCreds holds short-lived STS credentials for a break-glass AWS scope.
type BreakglassAWSCreds struct {
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token"`
	Expiration      time.Time `json:"expiration"`
	Region          string    `json:"region"`
	RoleARN         string    `json:"role_arn"`
}

func (e *Executor) breakglassAWSRoleARN(scopeID string) (string, error) {
	def, ok := breakglass.Get(scopeID)
	if !ok {
		return "", fmt.Errorf("unknown scope %q", scopeID)
	}
	switch def.Metadata.AwsRoleKey {
	case breakglass.AwsRoleKeyRead:
		if e.breakglassAWSReadARN == "" {
			return "", fmt.Errorf("BREAKGLASS_AWS_READ_ROLE_ARN not configured")
		}
		return e.breakglassAWSReadARN, nil
	case breakglass.AwsRoleKeyWrite:
		if e.breakglassAWSWriteARN == "" {
			return "", fmt.Errorf("BREAKGLASS_AWS_WRITE_ROLE_ARN not configured")
		}
		return e.breakglassAWSWriteARN, nil
	case breakglass.AwsRoleKeyElevated:
		if e.breakglassAWSElevatedARN == "" {
			return "", fmt.Errorf("BREAKGLASS_AWS_ELEVATED_ROLE_ARN not configured")
		}
		return e.breakglassAWSElevatedARN, nil
	default:
		return "", fmt.Errorf("scope %q has no AWS role key", scopeID)
	}
}

// AssumeBreakglassRole issues STS credentials for an approved break-glass grant.
func (e *Executor) AssumeBreakglassRole(ctx context.Context, requestID, scopeID string, duration time.Duration) (*BreakglassAWSCreds, error) {
	if e.stsClient == nil {
		return nil, fmt.Errorf("STS client not configured")
	}
	roleARN, err := e.breakglassAWSRoleARN(scopeID)
	if err != nil {
		return nil, err
	}
	secs := int32(duration.Seconds())
	if secs < 900 {
		secs = 900
	}
	if secs > 3600 {
		secs = 3600
	}
	out, err := e.stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(fmt.Sprintf("zoa-bg-%s", requestID)),
		DurationSeconds: aws.Int32(secs),
	})
	if err != nil {
		return nil, fmt.Errorf("assuming break-glass role: %w", err)
	}
	return &BreakglassAWSCreds{
		AccessKeyID:     aws.ToString(out.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(out.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(out.Credentials.SessionToken),
		Expiration:      aws.ToTime(out.Credentials.Expiration),
		Region:          e.region,
		RoleARN:         roleARN,
	}, nil
}
