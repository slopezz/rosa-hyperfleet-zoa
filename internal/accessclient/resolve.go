// Package accessclient resolves ZOA deployment SSM entries and transparently
// assumes the invoker role so the CLI can reach the Access Lambda without
// manual sts:AssumeRole steps.
package accessclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ssmDeploymentsPath is the SSM path prefix in the Central Account.
const ssmDeploymentsPath = "/zoa/deployments"

// DeploymentInfo holds the resolved credentials and metadata for a deployment.
type DeploymentInfo struct {
	AccessURL   string                  // Access Lambda Function URL
	Credentials aws.CredentialsProvider // Invoker-role assumed credentials
	Region      string                  // Deployment AWS region
	AccountID   string                  // Deployment AWS account
	SessionName string                  // SRE username (RoleSessionName on invoker role)
	OperatorARN string                  // Full STS ARN after assuming invoker (for X-Operator)
}

// ssmDeployment mirrors the JSON stored in SSM by Terraform.
type ssmDeployment struct {
	DeploymentName string `json:"deployment_name"`
	AccessURL      string `json:"access_url"`
	InvokerRoleARN string `json:"invoker_role_arn"`
	Region         string `json:"region"`
	AccountID      string `json:"account_id"`
}

// Resolve reads the SSM deployment entry, extracts the SRE's identity from
// the current caller ARN, and assumes the deployment's invoker role.
//
// The caller's existing AWS credentials (e.g. Central Account via rh-aws-saml-login)
// are used for SSM read and STS AssumeRole. The returned DeploymentInfo contains
// temporary credentials scoped to the invoker role and the Access Lambda URL.
func Resolve(ctx context.Context, deploymentName string) (*DeploymentInfo, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	dep, err := readDeploymentSSM(ctx, cfg, deploymentName)
	if err != nil {
		return nil, err
	}

	sessionName, err := extractSessionName(ctx, cfg)
	if err != nil {
		return nil, err
	}

	creds := assumeInvokerRole(cfg, dep.InvokerRoleARN, sessionName)

	operatorARN, err := callerARN(ctx, cfg, creds)
	if err != nil {
		return nil, fmt.Errorf("invoker role identity: %w", err)
	}

	return &DeploymentInfo{
		AccessURL:   dep.AccessURL,
		Credentials: creds,
		Region:      dep.Region,
		AccountID:   dep.AccountID,
		SessionName: sessionName,
		OperatorARN: operatorARN,
	}, nil
}

// readDeploymentSSM fetches a single deployment entry from SSM.
func readDeploymentSSM(ctx context.Context, cfg aws.Config, name string) (*ssmDeployment, error) {
	ssmClient := ssm.NewFromConfig(cfg)

	paramName := ssmDeploymentsPath + "/" + name
	out, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(paramName),
	})
	if err != nil {
		return nil, fmt.Errorf("reading SSM %s: %w", paramName, err)
	}

	var dep ssmDeployment
	if err := json.Unmarshal([]byte(aws.ToString(out.Parameter.Value)), &dep); err != nil {
		return nil, fmt.Errorf("parsing SSM parameter %s: %w", paramName, err)
	}

	if dep.AccessURL == "" {
		return nil, fmt.Errorf("deployment %q has no access_url in SSM", name)
	}
	if dep.InvokerRoleARN == "" {
		return nil, fmt.Errorf("deployment %q has no invoker_role_arn in SSM", name)
	}

	return &dep, nil
}

// extractSessionName gets the current caller identity and extracts the
// trailing identity segment from the ARN.
//
// Supported ARN formats:
//
//	arn:aws:sts::ACCOUNT:assumed-role/ROLE/SESSION_NAME → SESSION_NAME
//	arn:aws:iam::ACCOUNT:user/USERNAME                 → USERNAME
//	arn:aws:iam::ACCOUNT:user/path/USERNAME            → USERNAME
func extractSessionName(ctx context.Context, cfg aws.Config) (string, error) {
	stsClient := sts.NewFromConfig(cfg)

	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("getting caller identity: %w", err)
	}

	arn := aws.ToString(identity.Arn)
	name := extractNameFromARN(arn)
	if name == "" {
		return "", fmt.Errorf("cannot extract session name from ARN %q", arn)
	}

	return name, nil
}

// extractNameFromARN returns the trailing identity segment from an IAM/STS ARN.
// Returns empty string if the ARN format is not recognized.
func extractNameFromARN(arn string) string {
	// Split on ":" to get the resource part (last segment).
	// arn:aws:sts::ACCOUNT:assumed-role/ROLE/SESSION_NAME
	// arn:aws:iam::ACCOUNT:user/USERNAME
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return ""
	}
	resource := parts[5]

	// Take the last "/" segment.
	segments := strings.Split(resource, "/")
	if len(segments) < 2 {
		return ""
	}
	return segments[len(segments)-1]
}

// assumeInvokerRole returns a CredentialsProvider that lazily assumes the
// invoker role with the given session name.
func assumeInvokerRole(cfg aws.Config, roleARN, sessionName string) aws.CredentialsProvider {
	stsClient := sts.NewFromConfig(cfg)
	return stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = sessionName
	})
}

// callerARN returns GetCallerIdentity.Arn for the given credential provider.
func callerARN(ctx context.Context, baseCfg aws.Config, creds aws.CredentialsProvider) (string, error) {
	cfg := baseCfg
	cfg.Credentials = aws.NewCredentialsCache(creds)
	stsClient := sts.NewFromConfig(cfg)
	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("getting caller identity: %w", err)
	}
	arn := aws.ToString(out.Arn)
	if arn == "" {
		return "", fmt.Errorf("empty caller ARN")
	}
	return arn, nil
}
