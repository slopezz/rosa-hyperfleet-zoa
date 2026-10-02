package accesscross

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/api"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/awsecs"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/targetroles"
)

// NewECSFactory returns an ECS client factory that assumes boundary-access in MC accounts.
func NewECSFactory(homeCfg aws.Config, homeAccountID string, local *awsecs.Client) api.ECSClientFactory {
	return func(ctx context.Context, target *store.Target) (api.ECSAPI, error) {
		if local == nil {
			return nil, fmt.Errorf("ecs client not configured")
		}
		if target == nil || target.AccountId == "" || homeAccountID == "" || target.AccountId == homeAccountID {
			return local, nil
		}
		roleARN := targetroles.BoundaryAccessRoleARN(target.AccountId, target.TargetID)
		cfg, err := assumeRole(ctx, homeCfg, target.Region, roleARN)
		if err != nil {
			return nil, err
		}
		return awsecs.New(cfg), nil
	}
}

func assumeRole(ctx context.Context, homeCfg aws.Config, region, roleARN string) (aws.Config, error) {
	stsClient := sts.NewFromConfig(homeCfg)
	creds := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = "zoa-access-boundary"
	})
	targetRegion := region
	if targetRegion == "" {
		targetRegion = homeCfg.Region
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(targetRegion),
		awsconfig.WithCredentialsProvider(aws.NewCredentialsCache(creds)),
	)
	if err != nil {
		return cfg, fmt.Errorf("load AWS config for role %s: %w", roleARN, err)
	}
	return cfg, nil
}
