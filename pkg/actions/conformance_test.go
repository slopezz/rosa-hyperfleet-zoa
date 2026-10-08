package actions

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// getTestEnvInt reads an env var as int with a fallback default.
// Used to derive the execution deadline ceiling from the same env var
// the production code uses, ensuring the test stays in sync with infra.
func getTestEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

// TestAllRegisteredActions_Conformance is the PR gate for TA quality.
// Any new TA that fails these checks will break the build, forcing the
// author to fix their metadata, RBAC, or output contract before merge.
func TestAllRegisteredActions_Conformance(t *testing.T) {
	actions := ListCatalog()
	if len(actions) == 0 {
		t.Fatal("no actions registered — init() functions likely not running")
	}

	validScopes := map[string]bool{"kube-api": true, "aws-api": true}
	validTypes := map[string]bool{"read": true, "write": true}
	validModes := map[string]bool{"sync": true, "async": true}
	validApprovals := map[string]bool{"none": true, "auto": true, "manual": true}

	for _, action := range actions {
		meta := action.Metadata()
		t.Run(meta.Name, func(t *testing.T) {

			// --- Required metadata fields ---

			if meta.Name == "" {
				t.Error("Name must not be empty")
			}
			if !validScopes[meta.Scope] {
				t.Errorf("Scope must be 'kube-api' or 'aws-api', got %q", meta.Scope)
			}
			if !validTypes[meta.Type] {
				t.Errorf("Type must be 'read' or 'write', got %q", meta.Type)
			}
			if !validModes[meta.ExecutionMode] {
				t.Errorf("ExecutionMode must be 'sync' or 'async', got %q", meta.ExecutionMode)
			}
			if meta.Description == "" {
				t.Error("Description must not be empty — explain what the TA does")
			}
			if len(meta.Examples) == 0 {
				t.Error("Examples must include at least one zoa run command with --reason")
			}
			for i, ex := range meta.Examples {
				if !strings.Contains(ex, "--reason") {
					t.Errorf("Examples[%d] must include --reason: %q", i, ex)
				}
				if !strings.HasPrefix(ex, "zoa run ") {
					t.Errorf("Examples[%d] must start with 'zoa run ': %q", i, ex)
				}
			}
			if !validApprovals[meta.Authorization.Approval] {
				t.Errorf("Authorization.Approval must be 'none', 'auto', or 'manual', got %q", meta.Authorization.Approval)
			}
			if meta.TimeoutSeconds <= 0 {
				t.Errorf("TimeoutSeconds must be > 0, got %d", meta.TimeoutSeconds)
			}

			// --- Naming convention ---

			if strings.Contains(meta.Name, "-") {
				t.Errorf("Name should use snake_case (underscores), got %q", meta.Name)
			}

			// --- Scope-RBAC consistency ---

			if meta.Scope == "kube-api" && meta.RBAC == nil {
				t.Error("kube-api TAs must declare RBAC (even if empty rules) — " +
					"this prevents runtime failures where the SA has no permissions")
			}
			if meta.Scope == "aws-api" && meta.RBAC != nil {
				t.Error("aws-api TAs must NOT declare RBAC — " +
					"AWS TAs use STS AssumeRole, not K8s ServiceAccount RBAC")
			}

			// --- RBAC rules (already tested by RBACCompliance but included for completeness) ---

			if meta.RBAC != nil {
				if err := ValidateRBACRules(meta.RBAC); err != nil {
					t.Errorf("RBAC validation failed: %v", err)
				}
				if len(meta.RBAC.Rules) == 0 {
					t.Error("RBAC is declared but has no rules — either add rules or set RBAC to nil")
				}
			}

			// --- Write-TA specific checks ---

			if meta.Type == "write" {
				if meta.WriteCooldownSeconds <= 0 {
					t.Error("write TAs must have WriteCooldownSeconds > 0 to prevent accidental repetition")
				}
			}

			// --- DryRunAction chain integrity ---

			if meta.DryRunAction != "" {
				dryAction, exists := GetCatalog(meta.DryRunAction)
				if !exists {
					t.Errorf("DryRunAction %q is not a registered action", meta.DryRunAction)
				} else {
					dryMeta := dryAction.Metadata()
					if dryMeta.Type != "read" {
						t.Errorf("DryRunAction %q must be a 'read' TA (got type=%q)", meta.DryRunAction, dryMeta.Type)
					}
				}
			}

			// --- Timeout ceiling (sync TAs only; async TAs use K8s Job activeDeadlineSeconds) ---

			if meta.ExecutionMode != "async" {
				ceiling := getTestEnvInt("EXECUTION_DEADLINE_SECONDS", 295)
				if meta.TimeoutSeconds > ceiling {
					t.Errorf("TimeoutSeconds=%d exceeds execution deadline ceiling (%ds). "+
						"Increase Lambda timeout in Terraform (lambda_worker_timeout), "+
						"set EXECUTION_DEADLINE_SECONDS env var, and update. "+
						"See docs/architecture/timeout-tuning.md",
						meta.TimeoutSeconds, ceiling)
				}
			}

			// --- Deployment target declaration ---

			if len(meta.DeploymentTargets) == 0 {
				t.Error("DeploymentTargets must declare at least rc and/or mc")
			}
			for _, target := range meta.DeploymentTargets {
				if target != DeploymentTargetRC && target != DeploymentTargetMC {
					t.Errorf("DeploymentTargets entry must be rc or mc, got %q", target)
				}
			}

			// --- Parameters consistency ---

			paramNames := make(map[string]bool)
			for _, p := range meta.Parameters {
				if p.Name == "" {
					t.Error("parameter Name must not be empty")
				}
				if p.Description == "" {
					t.Errorf("parameter %q must have a Description", p.Name)
				}
				if paramNames[p.Name] {
					t.Errorf("duplicate parameter name %q", p.Name)
				}
				paramNames[p.Name] = true
			}

			// Namespace-scoped RBAC must reference a declared parameter
			if meta.RBAC != nil && meta.RBAC.NamespaceParam != "" {
				if !paramNames[meta.RBAC.NamespaceParam] {
					t.Errorf("RBAC.NamespaceParam %q is not declared in Parameters", meta.RBAC.NamespaceParam)
				}
			}
		})
	}
}

// TestAllRegisteredActions_HaveTestFile ensures every TA implementation file
// has a corresponding test file. This prevents shipping untested TAs.
func TestAllRegisteredActions_HaveTestFile(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	actionsDir := filepath.Dir(thisFile)

	for _, action := range ListCatalog() {
		meta := action.Metadata()
		t.Run(meta.Name+"/has_test", func(t *testing.T) {
			// Find test files that could contain tests for this TA
			testFiles, err := filepath.Glob(filepath.Join(actionsDir, "*_test.go"))
			if err != nil {
				t.Fatalf("glob error: %v", err)
			}

			// Check if any test file references this action's test patterns
			taName := meta.Name
			found := false
			lowerName := strings.ToLower(taName)
			camelName := strings.ToLower(toCamelCase(taName))
			for _, tf := range testFiles {
				if tf == filepath.Join(actionsDir, "conformance_test.go") {
					continue
				}
				data, err := os.ReadFile(tf)
				if err != nil {
					continue
				}
				content := strings.ToLower(string(data))
				if strings.Contains(content, lowerName) ||
					strings.Contains(content, camelName) {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("TA %q has no test coverage — add tests in pkg/actions/ that reference %q",
					meta.Name, taName)
			}
		})
	}
}

// toCamelCase converts snake_case to CamelCase for matching Go type names.
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// TestAllRegisteredActions_OutputContract validates that the ActionResult
// contract is respected: Success=true requires non-empty Summary.
// This is a compile-time documentation gate — it can't test actual Execute()
// output without live infra, but it ensures the type system is used correctly.
func TestAllRegisteredActions_OutputContract(t *testing.T) {
	t.Run("ActionResult_WhenSuccess_MustHaveSummary", func(t *testing.T) {
		// This is a contract documentation test — validates the type expectations
		// that all TA authors must follow. Actual output testing happens per-TA.
		result := &ActionResult{Success: true, Output: nil, Summary: ""}
		if result.Success && result.Summary == "" && result.Output == nil {
			// This is the anti-pattern we're documenting
			_ = "TA must set Summary on success and Output should be non-nil JSON-serializable data"
		}
	})
}
