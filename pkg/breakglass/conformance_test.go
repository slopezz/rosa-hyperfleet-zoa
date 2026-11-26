package breakglass

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

// shippedApproval is the only approval policy in the current break-glass implementation
// (see docs/approval-workflow.md). When the workflow epic lands, extend validApprovals here.
const shippedApproval = "none"

var validScopes = map[string]bool{ScopeKube: true, ScopeAWS: true}
var validTypes = map[string]bool{TypeRead: true, TypeWrite: true, TypeElevated: true}

func TestAllRegisteredProfiles_Conformance(t *testing.T) {
	SetTargetType("")
	defs := ListDefinitions()
	if len(defs) == 0 {
		t.Fatal("no profiles registered — init() functions likely not running")
	}

	for _, def := range defs {
		meta := def.Metadata
		t.Run(meta.Name, func(t *testing.T) {
			if !validScopes[meta.Scope] {
				t.Errorf("scope must be kube or aws, got %q", meta.Scope)
			}
			if !validTypes[meta.Type] {
				t.Errorf("type must be read, write, or elevated, got %q", meta.Type)
			}
			if strings.Contains(meta.Name, "-") {
				t.Errorf("Name should use snake_case (underscores), got %q", meta.Name)
			}
			if meta.Authorization.Approval != shippedApproval {
				t.Errorf("authorization.approval must be %q until workflow epic (got %q)", shippedApproval, meta.Authorization.Approval)
			}
			if err := ValidateProfile(def); err != nil {
				t.Error(err)
			}
			if len(meta.Examples) == 0 {
				t.Error("Examples must include at least one zoa breakglass request command with --reason")
			}
			for i, ex := range meta.Examples {
				if !strings.Contains(ex, "--reason") {
					t.Errorf("Examples[%d] must include --reason: %q", i, ex)
				}
				if !strings.HasPrefix(ex, "zoa breakglass request ") {
					t.Errorf("Examples[%d] must start with 'zoa breakglass request ': %q", i, ex)
				}
			}
		})
	}
}

func TestAllRegisteredProfiles_RBACCompliance(t *testing.T) {
	for _, err := range ValidateAllProfiles() {
		t.Error(err)
	}
}

func TestAllRegisteredProfiles_HaveTestFile(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	testFiles, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	skip := map[string]bool{
		"conformance_test.go":    true,
		"approval_test.go":       true,
		"rbac_validator_test.go": true,
	}

	for _, def := range ListDefinitions() {
		meta := def.Metadata
		t.Run(meta.Name+"/has_test", func(t *testing.T) {
			wantStem := strings.ReplaceAll(meta.Name, "-", "_")
			found := false
			for _, tf := range testFiles {
				base := filepath.Base(tf)
				if skip[base] {
					continue
				}
				if strings.HasPrefix(base, wantStem) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("profile %q expects pkg/breakglass/%s_test.go", meta.Name, wantStem)
			}
		})
	}
}

func TestKubeProfiles_WhenRegistered_TheyShouldExposeRBACForRCAndMC(t *testing.T) {
	for _, def := range ListDefinitions() {
		if def.Metadata.Scope != ScopeKube {
			continue
		}
		t.Run(def.Metadata.Name, func(t *testing.T) {
			for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
				if rules := def.RBAC(target); len(rules) == 0 {
					t.Errorf("missing RBAC for target %s", target)
				}
			}
		})
	}
}

func TestAWSProfiles_WhenRegistered_TheyShouldUseKnownProfileNames(t *testing.T) {
	for _, id := range []string{ProfileAWSRead, ProfileAWSWrite, ProfileAWSElevated} {
		def, ok := Get(id)
		if !ok {
			t.Fatalf("profile %q not registered", id)
		}
		if def.Metadata.Name != id {
			t.Errorf("name: want %q, got %q", id, def.Metadata.Name)
		}
		if def.Metadata.Scope != ScopeAWS {
			t.Errorf("profile %q: scope want aws, got %q", id, def.Metadata.Scope)
		}
		if def.RBAC != nil {
			t.Error("aws profile must not set RBAC func")
		}
	}
}

// Ensure authoring layout: one implementation file per profile name (underscore form).
func TestProfileSourceFiles_WhenPresent_TheyShouldMatchCatalog(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	implStems := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		switch name {
		case "types.go", "registry.go", "approval.go", "rbac_validator.go", "aws_profiles.go",
			"kube_rbac.go", "describe.go", "credential.go":
			continue
		}
		implStems[strings.TrimSuffix(name, ".go")] = true
	}
	for _, def := range ListDefinitions() {
		stem := strings.ReplaceAll(def.Metadata.Name, "-", "_")
		if !implStems[stem] {
			t.Errorf("profile %q missing source file %s.go", def.Metadata.Name, stem)
		}
	}
}
