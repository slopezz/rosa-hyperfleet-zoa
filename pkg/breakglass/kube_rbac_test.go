package breakglass

import (
	"slices"
	"testing"
)

func TestKubeProfiles_WhenPlatformLists_ItShouldMatchAcrossProfiles(t *testing.T) {
	if !slices.Equal(kubeReadPlatformRCAPIGroups, kubeWritePlatformRCAPIGroups) ||
		!slices.Equal(kubeReadPlatformRCAPIGroups, kubeElevatedPlatformRCAPIGroups) {
		t.Fatal("RC platform apiGroups must match across kube_read, kube_write, kube_elevated")
	}
	if !slices.Equal(kubeReadPlatformMCAPIGroups, kubeWritePlatformMCAPIGroups) ||
		!slices.Equal(kubeReadPlatformMCAPIGroups, kubeElevatedPlatformMCAPIGroups) {
		t.Fatal("MC platform apiGroups must match across kube_read, kube_write, kube_elevated")
	}
}