package cli

import (
	"sort"
	"strings"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func registeredActionNames() []string {
	all := actions.ListCatalog()
	names := make([]string, 0, len(all))
	for _, a := range all {
		names = append(names, a.Metadata().Name)
	}
	sort.Strings(names)
	return names
}

func completeActionNames(toComplete string) []string {
	var out []string
	for _, name := range registeredActionNames() {
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name)
		}
	}
	return out
}

func actionParameterCompletions(actionName, toComplete string) []string {
	a, ok := actions.GetCatalog(actionName)
	if !ok {
		return nil
	}
	meta := a.Metadata()
	var out []string
	for _, p := range meta.Parameters {
		suggestion := p.Name + "="
		if strings.HasPrefix(suggestion, toComplete) || strings.HasPrefix(p.Name, toComplete) {
			out = append(out, suggestion)
		}
	}
	return out
}
