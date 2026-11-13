package cli

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
)

// loadBoundaryAPIAWSConfig loads credentials for ZOA API SigV4 from inside the boundary task.
// Break-glass activate must not affect Lambda invokes: only the ECS task role is used — not
// break-glass profiles/files or ambient AWS_* access-key environment variables.
func loadBoundaryAPIAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	_ = ctx
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
	}
	return aws.Config{
		Region:      region,
		Credentials: aws.NewCredentialsCache(ec2rolecreds.New()),
	}, nil
}
