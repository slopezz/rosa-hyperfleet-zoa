package cli

import (
	"context"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/client"
)

func TestDescribeBreakglassProfile_WhenProfileExists_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getBreakglassProfileFn: func(_ context.Context, name string) (*client.ProfileMetadata, error) {
			if name != "kube_read" {
				t.Fatalf("expected kube_read, got %q", name)
			}
			return &client.ProfileMetadata{
				Name:        "kube_read",
				Scope:       "kube",
				Type:        "read",
				Description: "read-only kube",
			}, nil
		},
	}
	global := newMockGlobalOpts(mock)
	err := describeBreakglassProfile(context.Background(), global, &catalogOptions{}, "kube_read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}