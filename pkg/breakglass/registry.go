package breakglass

import (
	"fmt"
	"sort"
	"sync"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

var (
	mu            sync.RWMutex
	catalog       = make(map[string]*Definition)
	runtimeTarget string
)

// SetTargetType filters profiles to the ZOA_TARGET_TYPE (rc or mc) at runtime.
func SetTargetType(target string) {
	mu.Lock()
	defer mu.Unlock()
	runtimeTarget = target
}

func register(def *Definition) {
	mu.Lock()
	defer mu.Unlock()
	id := def.Metadata.Name
	if _, exists := catalog[id]; exists {
		panic(fmt.Sprintf("profile %q already registered", id))
	}
	catalog[id] = def
}

func allowedOnRuntimeTarget(meta ProfileMetadata) bool {
	target := runtimeTarget
	if target == "" || len(meta.TargetTypes) == 0 {
		return true
	}
	for _, t := range meta.TargetTypes {
		if t == target {
			return true
		}
	}
	return false
}

// Get returns a profile visible on this target type.
func Get(profileID string) (*Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	def, ok := catalog[profileID]
	if !ok || !allowedOnRuntimeTarget(def.Metadata) {
		return nil, false
	}
	return def, true
}

// ListDefinitions returns registered profiles for this target type, sorted by name.
func ListDefinitions() []*Definition {
	mu.RLock()
	defer mu.RUnlock()
	var out []*Definition
	for _, def := range catalog {
		if allowedOnRuntimeTarget(def.Metadata) {
			out = append(out, def)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metadata.Name < out[j].Metadata.Name })
	return out
}

// List returns registered profile metadata for this target type, sorted by name.
func List() []ProfileMetadata {
	mu.RLock()
	defer mu.RUnlock()
	var out []ProfileMetadata
	for _, def := range catalog {
		if allowedOnRuntimeTarget(def.Metadata) {
			out = append(out, def.Metadata)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// KubeReadRules returns RBAC rules for kube_read on RC or MC.
func KubeReadRules(targetType string) []actions.RBACRule {
	def, ok := Get("kube_read")
	if !ok || def.RBAC == nil {
		return nil
	}
	return def.RBAC(targetType)
}
