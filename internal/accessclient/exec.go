package accessclient

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	// DefaultExecRoleName is assumed in the deployment account for ECS Exec.
	// Central (or jump) credentials must be allowed to sts:AssumeRole this role in account_id from SSM.
	DefaultExecRoleName = "OrganizationAccountAccessRole"

	execAWSProfileEnv = "ZOA_EXEC_AWS_PROFILE"
	execRoleNameEnv   = "ZOA_EXEC_ROLE_NAME"
)

// ExecAWSConfig returns AWS config scoped to the deployment account for ECS Exec.
//
// By default, uses the caller's ambient credentials (Central Account) to read
// /zoa/deployments/<name> and assume OrganizationAccountAccessRole in the deployment
// account_id. Set ZOA_EXEC_AWS_PROFILE to use a fixed profile instead (local override).
func ExecAWSConfig(ctx context.Context, deploymentName string) (aws.Config, error) {
	if profile := os.Getenv(execAWSProfileEnv); profile != "" {
		cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithSharedConfigProfile(profile))
		if err != nil {
			return cfg, fmt.Errorf("loading AWS profile %q for ECS Exec: %w", profile, err)
		}
		return cfg, nil
	}

	baseCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return baseCfg, fmt.Errorf("loading AWS config: %w", err)
	}

	dep, err := readDeploymentSSM(ctx, baseCfg, deploymentName)
	if err != nil {
		return baseCfg, err
	}
	if dep.AccountID == "" {
		return baseCfg, fmt.Errorf("deployment %q has no account_id in SSM", deploymentName)
	}
	if dep.Region == "" {
		return baseCfg, fmt.Errorf("deployment %q has no region in SSM", deploymentName)
	}

	sessionName, err := extractSessionName(ctx, baseCfg)
	if err != nil {
		return baseCfg, err
	}

	roleName := os.Getenv(execRoleNameEnv)
	if roleName == "" {
		roleName = DefaultExecRoleName
	}
	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", dep.AccountID, roleName)

	stsClient := sts.NewFromConfig(baseCfg)
	execCreds := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = sessionName
	})

	cfg := baseCfg
	cfg.Region = dep.Region
	cfg.Credentials = aws.NewCredentialsCache(execCreds)
	return cfg, nil
}
