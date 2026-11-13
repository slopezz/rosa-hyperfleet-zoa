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

var validPlanes = map[string]bool{PlaneKube: true, PlaneAWS: true}
var validTypes = map[string]bool{TypeRead: true, TypeWrite: true, TypeElevated: true}

func TestAllRegisteredScopes_Conformance(t *testing.T) {
	SetTargetType("")
	defs := ListDefinitions()
	if len(defs) == 0 {
		t.Fatal("no scopes registered — init() functions likely not running")
	}

	for _, def := range defs {
		meta := def.Metadata
		t.Run(meta.ID, func(t *testing.T) {
			if !validPlanes[meta.Plane] {
				t.Errorf("plane must be kube or aws, got %q", meta.Plane)
			}
			if !validTypes[meta.Type] {
				t.Errorf("type must be read, write, or elevated, got %q", meta.Type)
			}
			if meta.Authorization.Approval != shippedApproval {
				t.Errorf("authorization.approval must be %q until workflow epic (got %q)", shippedApproval, meta.Authorization.Approval)
			}
			if err := ValidateScope(def); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestAllRegisteredScopes_RBACCompliance(t *testing.T) {
	for _, err := range ValidateAllScopes() {
		t.Error(err)
	}
}

func TestAllRegisteredScopes_HaveTestFile(t *testing.T) {
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
		t.Run(meta.ID+"/has_test", func(t *testing.T) {
			wantStem := strings.ReplaceAll(meta.ID, "-", "_")
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
				t.Errorf("scope %q expects pkg/breakglass/%s_test.go", meta.ID, wantStem)
			}
		})
	}
}

func TestKubeScopes_WhenRegistered_TheyShouldExposeRBACForRCAndMC(t *testing.T) {
	for _, def := range ListDefinitions() {
		if def.Metadata.Plane != PlaneKube {
			continue
		}
		t.Run(def.Metadata.ID, func(t *testing.T) {
			for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
				if rules := def.RBAC(target); len(rules) == 0 {
					t.Errorf("missing RBAC for target %s", target)
				}
			}
		})
	}
}

func TestAWSScopes_WhenRegistered_TheyShouldSetRoleKeys(t *testing.T) {
	expected := map[string]string{
		"aws-read":     AwsRoleKeyRead,
		"aws-write":    AwsRoleKeyWrite,
		"aws-elevated": AwsRoleKeyElevated,
	}
	for id, wantKey := range expected {
		def, ok := Get(id)
		if !ok {
			t.Fatalf("scope %q not registered", id)
		}
		if def.Metadata.AwsRoleKey != wantKey {
			t.Errorf("aws_role_key: want %q, got %q", wantKey, def.Metadata.AwsRoleKey)
		}
		if def.RBAC != nil {
			t.Error("aws scope must not set RBAC func")
		}
	}
}

// Ensure authoring layout: one implementation file per scope id (underscore form).
func TestScopeSourceFiles_WhenPresent_TheyShouldMatchCatalog(t *testing.T) {
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
		case "types.go", "registry.go", "approval.go", "rbac_validator.go", "aws_keys.go":
			continue
		}
		implStems[strings.TrimSuffix(name, ".go")] = true
	}
	for _, def := range ListDefinitions() {
		stem := strings.ReplaceAll(def.Metadata.ID, "-", "_")
		if !implStems[stem] {
			t.Errorf("scope %q missing source file %s.go", def.Metadata.ID, stem)
		}
	}
}
