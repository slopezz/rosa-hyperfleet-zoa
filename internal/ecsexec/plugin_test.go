package ecsexec

import (
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestPluginEnv_WhenRegionProvidedItShouldPinAWSRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	t.Setenv("AWS_PROFILE", "central-dev")

	creds := aws.Credentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
	}

	env, err := pluginEnv("us-east-1", creds)
	if err != nil {
		t.Fatalf("pluginEnv: %v", err)
	}

	got := envMap(env)
	if got["AWS_REGION"] != "us-east-1" || got["AWS_DEFAULT_REGION"] != "us-east-1" {
		t.Fatalf("expected us-east-1 region pins, got AWS_REGION=%q AWS_DEFAULT_REGION=%q", got["AWS_REGION"], got["AWS_DEFAULT_REGION"])
	}
	if got["AWS_PROFILE"] != "" {
		t.Fatalf("expected AWS_PROFILE stripped, got %q", got["AWS_PROFILE"])
	}
	if got["AWS_ACCESS_KEY_ID"] != creds.AccessKeyID {
		t.Fatalf("expected injected access key")
	}
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}

func TestPluginEnv_WhenCredentialsMissingItShouldError(t *testing.T) {
	_, err := pluginEnv("us-east-1", aws.Credentials{})
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
}

func TestPluginEnv_WhenCalledItShouldNotMutateProcessEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "keep-me")
	_, err := pluginEnv("us-east-1", aws.Credentials{AccessKeyID: "a", SecretAccessKey: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("AWS_REGION") != "keep-me" {
		t.Fatalf("pluginEnv mutated process AWS_REGION")
	}
}
