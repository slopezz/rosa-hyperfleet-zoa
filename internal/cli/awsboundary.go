package cli

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// loadBoundaryAPIAWSConfig loads credentials for ZOA API SigV4 from inside the boundary task.
// Uses the ECS task role (container credentials env). Break-glass shell wiring must not
// affect Lambda invoke: static keys, profiles, and shared credential files are stripped
// before the default AWS SDK chain runs (ECS / IMDS remote providers remain).
func loadBoundaryAPIAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	clearBreakglassAWSFromProcessEnv()
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithSharedConfigFiles([]string{}),
		awsconfig.WithSharedCredentialsFiles([]string{}),
	}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

func clearBreakglassAWSFromProcessEnv() {
	for _, key := range []string{
		"AWS_PROFILE",
		"AWS_DEFAULT_PROFILE",
		"AWS_SHARED_CREDENTIALS_FILE",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
	} {
		_ = os.Unsetenv(key)
	}
}
