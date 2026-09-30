package cli

import (
	"testing"
)

func TestResolveDeploymentTarget(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		flagDeployment string
		flagTarget     string
		wantDeployment string
		wantTarget     string
	}{
		{
			name:           "When both positional args are given it should use them",
			args:           []string{"us-east-1", "mc01"},
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When positional args are given they should override flags",
			args:           []string{"us-east-1", "mc01"},
			flagDeployment: "eu-west-1",
			flagTarget:     "mc02",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When one positional arg is given it should be deployment with flag target",
			args:           []string{"us-east-1"},
			flagTarget:     "mc01",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When no positional args are given it should fall back to flags",
			args:           []string{},
			flagDeployment: "us-east-1",
			flagTarget:     "mc01",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When nothing is provided it should return empty strings",
			args:           []string{},
			wantDeployment: "",
			wantTarget:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment, target := resolveDeploymentTarget(tt.args, tt.flagDeployment, tt.flagTarget)
			if deployment != tt.wantDeployment {
				t.Errorf("deployment = %q, want %q", deployment, tt.wantDeployment)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
		})
	}
}
