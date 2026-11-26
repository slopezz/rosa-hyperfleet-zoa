package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// BreakglassAWSCreds holds short-lived STS credentials for a break-glass grant (aws credential scope).
type BreakglassAWSCreds struct {
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token"`
	Expiration      time.Time `json:"expiration"`
	Region          string    `json:"region"`
	RoleARN         string    `json:"role_arn"`
}

func (e *Executor) breakglassAWSRoleARN(profileID string) (string, error) {
	def, ok := breakglass.Get(profileID)
	if !ok {
		return "", fmt.Errorf("unknown break-glass profile %q", profileID)
	}
	switch def.Metadata.Name {
	case breakglass.ProfileAWSRead:
		if e.breakglassAWSReadARN == "" {
			return "", fmt.Errorf("BREAKGLASS_AWS_READ_ROLE_ARN not configured")
		}
		return e.breakglassAWSReadARN, nil
	case breakglass.ProfileAWSWrite:
		if e.breakglassAWSWriteARN == "" {
			return "", fmt.Errorf("BREAKGLASS_AWS_WRITE_ROLE_ARN not configured")
		}
		return e.breakglassAWSWriteARN, nil
	case breakglass.ProfileAWSElevated:
		if e.breakglassAWSElevatedARN == "" {
			return "", fmt.Errorf("BREAKGLASS_AWS_ELEVATED_ROLE_ARN not configured")
		}
		return e.breakglassAWSElevatedARN, nil
	default:
		return "", fmt.Errorf("profile %q is not aws_read, aws_write, or aws_elevated", profileID)
	}
}

// AssumeBreakglassRole issues STS credentials for an approved break-glass grant.
func (e *Executor) AssumeBreakglassRole(ctx context.Context, requestID, profileID string, duration time.Duration) (*BreakglassAWSCreds, error) {
	if e.stsClient == nil {
		return nil, fmt.Errorf("STS client not configured")
	}
	roleARN, err := e.breakglassAWSRoleARN(profileID)
	if err != nil {
		return nil, err
	}
	credTTL, err := breakglass.CredentialDurationForGrant(breakglass.ScopeAWS, duration)
	if err != nil {
		return nil, err
	}
	secs := int32(credTTL.Seconds())
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
