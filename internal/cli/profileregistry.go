package cli

import (
	"sort"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli/targettype"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

func registeredProfileNames() []string {
	targetType := targettype.ResolveOptional("")
	if targetType != "" {
		breakglass.SetTargetType(targetType)
		defer breakglass.SetTargetType("")
	}
	var names []string
	for _, p := range breakglass.List() {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

func completeProfileNames(toComplete string) []string {
	var out []string
	for _, name := range registeredProfileNames() {
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name)
		}
	}
	return out
}
