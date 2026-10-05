package cli

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func TestParseSSMDeploymentParameters_WhenValidJSON_ItShouldDecodeDeployments(t *testing.T) {
	params := []ssmtypes.Parameter{
		{
			Name: aws.String("/zoa/deployments/us-east-1"),
			Value: aws.String(`{
				"deployment_name": "us-east-1",
				"access_url": "https://example.lambda-url.us-east-1.on.aws",
				"invoker_role_arn": "arn:aws:iam::123:role/invoker",
				"region": "us-east-1",
				"account_id": "123"
			}`),
		},
	}

	deployments, err := parseSSMDeploymentParameters(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deployments))
	}
	if deployments[0].DeploymentName != "us-east-1" {
		t.Errorf("expected deployment name us-east-1, got %q", deployments[0].DeploymentName)
	}
	if deployments[0].AccessURL == "" {
		t.Error("expected access_url to be set")
	}
}

func TestParseSSMDeploymentParameters_WhenInvalidJSON_ItShouldReturnError(t *testing.T) {
	params := []ssmtypes.Parameter{
		{
			Name:  aws.String("/zoa/deployments/bad"),
			Value: aws.String("{not-json"),
		},
	}

	_, err := parseSSMDeploymentParameters(params)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
