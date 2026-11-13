package cli

import (
	"context"
	"os"
	"testing"
)

func TestLoadBoundaryAPIAWSConfig_WhenBreakglassEnvSet_ItShouldIgnoreSharedCredentialsFile(t *testing.T) {
	t.Setenv("AWS_PROFILE", "zoa-bg-should-not-use")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/fake-credentials")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAFAKE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "token")

	if _, err := loadBoundaryAPIAWSConfig(context.Background(), "us-east-1"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
		t.Fatal("expected break-glass env cleared for zoa API client")
	}
}

func TestLoadBoundaryAPIAWSConfig_WhenRegionUnset_ItShouldReadAWSRegionEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-west-1")
	cfg, err := loadBoundaryAPIAWSConfig(context.Background(), "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Region != "eu-west-1" {
		t.Fatalf("region = %q, want eu-west-1", cfg.Region)
	}
}
