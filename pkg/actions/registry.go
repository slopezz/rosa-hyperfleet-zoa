package actions

import (
	"fmt"
	"sort"
	"sync"
)

var (
	mu                      sync.RWMutex
	catalog                 = make(map[string]Action)
	runtimeDeploymentTarget string
)

// SetDeploymentTarget configures which ZOA target type (rc or mc) this process serves.
// Must be called once at startup from ZOA_TARGET_TYPE (or legacy ZOA_DEPLOYMENT_TARGET).
// When empty, all registered TAs remain visible (used in unit tests).
func SetDeploymentTarget(target string) {
	mu.Lock()
	defer mu.Unlock()
	runtimeDeploymentTarget = target
}

// DeploymentTarget returns the runtime deployment target set at startup.
func DeploymentTarget() string {
	mu.RLock()
	defer mu.RUnlock()
	return runtimeDeploymentTarget
}

func Register(a Action) {
	mu.Lock()
	defer mu.Unlock()

	name := a.Metadata().Name
	if _, exists := catalog[name]; exists {
		panic(fmt.Sprintf("action %q already registered", name))
	}
	catalog[name] = a
}

func allowedOnRuntimeTarget(meta ActionMetadata) bool {
	target := runtimeDeploymentTarget
	if target == "" {
		return true
	}
	for _, t := range meta.DeploymentTargets {
		if t == target {
			return true
		}
	}
	return false
}

// Get returns a registered action visible on this deployment target.
func Get(name string) (Action, bool) {
	mu.RLock()
	defer mu.RUnlock()

	a, ok := catalog[name]
	if !ok || !allowedOnRuntimeTarget(a.Metadata()) {
		return nil, false
	}
	return a, true
}

// GetCatalog returns an action from the full catalog, ignoring deployment target filtering.
func GetCatalog(name string) (Action, bool) {
	mu.RLock()
	defer mu.RUnlock()

	a, ok := catalog[name]
	return a, ok
}

// List returns registered actions visible on this deployment target.
func List() []Action {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]Action, 0, len(catalog))
	for _, a := range catalog {
		if allowedOnRuntimeTarget(a.Metadata()) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata().Name < out[j].Metadata().Name
	})
	return out
}

// ListCatalog returns all registered actions regardless of deployment target.
func ListCatalog() []Action {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]Action, 0, len(catalog))
	for _, a := range catalog {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata().Name < out[j].Metadata().Name
	})
	return out
}
