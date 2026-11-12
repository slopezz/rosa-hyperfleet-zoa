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

// SetTargetType filters scopes to the ZOA_TARGET_TYPE (rc or mc) at runtime.
func SetTargetType(target string) {
	mu.Lock()
	defer mu.Unlock()
	runtimeTarget = target
}

func register(def *Definition) {
	mu.Lock()
	defer mu.Unlock()
	id := def.Metadata.ID
	if _, exists := catalog[id]; exists {
		panic(fmt.Sprintf("scope %q already registered", id))
	}
	catalog[id] = def
}

func allowedOnRuntimeTarget(meta ScopeMetadata) bool {
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

// Get returns a scope visible on this target type.
func Get(id string) (*Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	def, ok := catalog[id]
	if !ok || !allowedOnRuntimeTarget(def.Metadata) {
		return nil, false
	}
	return def, true
}

// List returns registered scopes for this target type, sorted by id.
func List() []ScopeMetadata {
	mu.RLock()
	defer mu.RUnlock()
	var out []ScopeMetadata
	for _, def := range catalog {
		if allowedOnRuntimeTarget(def.Metadata) {
			out = append(out, def.Metadata)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// KubeReadRules returns RBAC rules for kube-read on RC or MC.
func KubeReadRules(targetType string) []actions.RBACRule {
	def, ok := Get("kube-read")
	if !ok || def.RBAC == nil {
		return nil
	}
	return def.RBAC(targetType)
}
