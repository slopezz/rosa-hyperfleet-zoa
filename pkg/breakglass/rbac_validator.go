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

// elevatedForbiddenDeleteResources must not receive delete even in kube_elevated (read-only via omission + guard).
var elevatedForbiddenDeleteResources = []string{
	"customresourcedefinitions",
	"namespaces",
	"mutatingwebhookconfigurations",
	"validatingwebhookconfigurations",
	"validatingadmissionpolicies",
	"validatingadmissionpolicybindings",
}

// ValidateProfile checks metadata shape and kube RBAC policy for a registered profile.
func ValidateProfile(def *Definition) error {
	if def == nil {
		return fmt.Errorf("nil profile definition")
	}
	meta := def.Metadata
	if err := validateProfileMetadata(meta); err != nil {
		return err
	}
	if err := validateAuthorization(meta); err != nil {
		return err
	}
	switch meta.Scope {
	case ScopeKube:
		return validateKubeProfile(def)
	case ScopeAWS:
		return validateAWSProfile(def)
	default:
		return fmt.Errorf("profile %s: scope must be %q or %q, got %q", meta.Name, ScopeKube, ScopeAWS, meta.Scope)
	}
}

func validateProfileMetadata(meta ProfileMetadata) error {
	if meta.Name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if meta.Description == "" {
		return fmt.Errorf("profile %s: description required", meta.Name)
	}
	switch meta.Type {
	case TypeRead, TypeWrite, TypeElevated:
	default:
		return fmt.Errorf("profile %s: type must be read, write, or elevated", meta.Name)
	}
	return nil
}

func validateAuthorization(meta ProfileMetadata) error {
	auth := meta.Authorization
	if auth.Approval == "" {
		return fmt.Errorf("profile %s: authorization.approval required", meta.Name)
	}
	if auth.DefaultGrantHours <= 0 {
		return fmt.Errorf("profile %s: default_grant_hours must be > 0", meta.Name)
	}
	if auth.DefaultRequestHours <= 0 {
		return fmt.Errorf("profile %s: default_request_hours must be > 0", meta.Name)
	}
	if meta.Type == TypeElevated && auth.DefaultGrantHours > 2 {
		return fmt.Errorf("profile %s: elevated default_grant_hours must be <= 2", meta.Name)
	}
	return nil
}

func validateKubeProfile(def *Definition) error {
	meta := def.Metadata
	if def.RBAC == nil {
		return fmt.Errorf("profile %s: kube profiles must declare RBAC", meta.Name)
	}
	for _, target := range []string{actions.TargetTypeRC, actions.TargetTypeMC} {
		rules := EffectiveRBAC(def, target)
		if len(rules) == 0 {
			return fmt.Errorf("profile %s: no RBAC rules for target %s", meta.Name, target)
		}
		cfg := &actions.RBACConfig{Rules: rules}
		if meta.Type == TypeElevated {
			cfg.AllowSecretRead = true
		}
		if err := actions.ValidateRBACRules(cfg); err != nil {
			return fmt.Errorf("profile %s (%s): %w", meta.Name, target, err)
		}
		if err := validatePrivilegeTypeRBAC(meta.Type, rules); err != nil {
			return fmt.Errorf("profile %s (%s): %w", meta.Name, target, err)
		}
		if meta.Type == TypeElevated {
			if err := validateElevatedRBAC(rules); err != nil {
				return fmt.Errorf("profile %s (%s): %w", meta.Name, target, err)
			}
		}
	}
	return nil
}

func validateAWSProfile(def *Definition) error {
	meta := def.Metadata
	if def.RBAC != nil {
		return fmt.Errorf("profile %s: aws profiles must not declare RBAC in Go", meta.Name)
	}
	if !KnownAWSProfiles[meta.Name] {
		return fmt.Errorf("profile %s: aws profiles must use name aws_read, aws_write, or aws_elevated", meta.Name)
	}
	return nil
}

func validatePrivilegeTypeRBAC(privilegeType string, rules []actions.RBACRule) error {
	for _, rule := range rules {
		for _, verb := range rule.Verbs {
			if privilegeType == TypeRead && writeVerbs[verb] {
				return fmt.Errorf("read profile must not include write verb %q", verb)
			}
		}
		if err := validateSecretsPolicy(privilegeType, rule); err != nil {
			return err
		}
		if privilegeType != TypeElevated && ruleAllowsPodsExec(rule) {
			return fmt.Errorf("only elevated profiles may grant pods/exec")
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

func validateSecretsPolicy(privilegeType string, rule actions.RBACRule) error {
	if !slices.Contains(rule.Resources, "secrets") {
		return nil
	}
	if privilegeType != TypeElevated {
		return fmt.Errorf("secrets only allowed on elevated profiles (type %q)", TypeElevated)
	}
	for _, verb := range rule.Verbs {
		if writeVerbs[verb] {
			return fmt.Errorf("secret write verbs are forbidden in break-glass")
		}
	}
	return nil
}

func validateElevatedRBAC(rules []actions.RBACRule) error {
	for _, rule := range rules {
		if slices.Contains(rule.Resources, "*") && slices.Contains(rule.Verbs, "delete") {
			return fmt.Errorf("elevated profile must not grant delete with resources *")
		}
		for _, res := range rule.Resources {
			if !slices.Contains(elevatedForbiddenDeleteResources, res) {
				continue
			}
			if slices.Contains(rule.Verbs, "delete") {
				return fmt.Errorf("elevated profile must not grant delete on %s", res)
			}
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

// ValidateAllProfiles returns validation errors for every registered profile (both RC and MC rules).
func ValidateAllProfiles() []error {
	var errs []error
	for _, def := range ListDefinitions() {
		if err := ValidateProfile(def); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
