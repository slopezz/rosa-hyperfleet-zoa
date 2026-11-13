package breakglass

import (
	"fmt"
	"slices"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"
)

var writeVerbs = map[string]bool{
	"create": true, "update": true, "patch": true, "delete": true,
}

var rbacBindResources = []string{
	"roles", "rolebindings", "clusterroles", "clusterrolebindings",
}

// ValidateScope checks metadata shape and kube RBAC policy for a registered scope.
func ValidateScope(def *Definition) error {
	if def == nil {
		return fmt.Errorf("nil scope definition")
	}
	meta := def.Metadata
	if err := validateScopeMetadata(meta); err != nil {
		return err
	}
	if err := validateAuthorization(meta); err != nil {
		return err
	}
	switch meta.Plane {
	case PlaneKube:
		return validateKubeScope(def)
	case PlaneAWS:
		return validateAWSScope(def)
	default:
		return fmt.Errorf("scope %s: plane must be %q or %q, got %q", meta.ID, PlaneKube, PlaneAWS, meta.Plane)
	}
}

func validateScopeMetadata(meta ScopeMetadata) error {
	if meta.ID == "" {
		return fmt.Errorf("scope id must not be empty")
	}
	if meta.Description == "" {
		return fmt.Errorf("scope %s: description required", meta.ID)
	}
	switch meta.Type {
	case TypeRead, TypeWrite, TypeElevated:
	default:
		return fmt.Errorf("scope %s: type must be read, write, or elevated", meta.ID)
	}
	return nil
}

func validateAuthorization(meta ScopeMetadata) error {
	auth := meta.Authorization
	if auth.Approval == "" {
		return fmt.Errorf("scope %s: authorization.approval required", meta.ID)
	}
	if auth.DefaultGrantHours <= 0 {
		return fmt.Errorf("scope %s: default_grant_hours must be > 0", meta.ID)
	}
	if auth.DefaultPendingHours <= 0 {
		return fmt.Errorf("scope %s: default_pending_hours must be > 0", meta.ID)
	}
	if meta.Type == TypeElevated && auth.DefaultGrantHours > 2 {
		return fmt.Errorf("scope %s: elevated default_grant_hours must be <= 2", meta.ID)
	}
	return nil
}

func validateKubeScope(def *Definition) error {
	meta := def.Metadata
	if meta.AwsRoleKey != "" {
		return fmt.Errorf("scope %s: kube scopes must not set aws_role_key", meta.ID)
	}
	if def.RBAC == nil {
		return fmt.Errorf("scope %s: kube scopes must declare RBAC", meta.ID)
	}
	for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
		rules := def.RBAC(target)
		if len(rules) == 0 {
			return fmt.Errorf("scope %s: no RBAC rules for target %s", meta.ID, target)
		}
		cfg := &actions.RBACConfig{Rules: rules}
		if meta.Type == TypeElevated {
			cfg.AllowSecretRead = true
		}
		if err := actions.ValidateRBACRules(cfg); err != nil {
			return fmt.Errorf("scope %s (%s): %w", meta.ID, target, err)
		}
		if err := validateScopeTypeRBAC(meta.Type, rules); err != nil {
			return fmt.Errorf("scope %s (%s): %w", meta.ID, target, err)
		}
	}
	return nil
}

func validateAWSScope(def *Definition) error {
	meta := def.Metadata
	if def.RBAC != nil {
		return fmt.Errorf("scope %s: aws scopes must not declare RBAC in Go", meta.ID)
	}
	if meta.AwsRoleKey == "" {
		return fmt.Errorf("scope %s: aws scopes require aws_role_key", meta.ID)
	}
	return nil
}

func validateScopeTypeRBAC(scopeType string, rules []actions.RBACRule) error {
	for _, rule := range rules {
		for _, verb := range rule.Verbs {
			if scopeType == TypeRead && writeVerbs[verb] {
				return fmt.Errorf("read scope must not include write verb %q", verb)
			}
		}
		if err := validateSecretsPolicy(scopeType, rule); err != nil {
			return err
		}
		if scopeType != TypeElevated && ruleAllowsPodsExec(rule) {
			return fmt.Errorf("only elevated scopes may grant pods/exec")
		}
		for _, res := range rule.Resources {
			if !slices.Contains(rbacBindResources, res) {
				continue
			}
			for _, verb := range rule.Verbs {
				if writeVerbs[verb] {
					return fmt.Errorf("RBAC bind mutations forbidden on %s", res)
				}
			}
		}
	}
	return nil
}

func validateSecretsPolicy(scopeType string, rule actions.RBACRule) error {
	if !slices.Contains(rule.Resources, "secrets") {
		return nil
	}
	if scopeType != TypeElevated {
		return fmt.Errorf("secrets only allowed in elevated scope")
	}
	for _, verb := range rule.Verbs {
		if writeVerbs[verb] {
			return fmt.Errorf("secret write verbs are forbidden in break-glass")
		}
	}
	return nil
}

func ruleAllowsPodsExec(rule actions.RBACRule) bool {
	if slices.Contains(rule.Resources, "pods/exec") {
		for _, v := range rule.Verbs {
			if v == "create" {
				return true
			}
		}
	}
	return false
}

// ValidateAllScopes returns validation errors for every registered scope (both RC and MC rules).
func ValidateAllScopes() []error {
	var errs []error
	for _, def := range ListDefinitions() {
		if err := ValidateScope(def); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
