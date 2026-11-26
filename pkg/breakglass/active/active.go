// Package active manages ~/.config/zoa/breakglass/active (kube/aws stanzas).
package active

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// Stanza is one active grant on a credential scope (kube or aws).
type Stanza struct {
	RequestID           string `json:"requestId"`
	Profile             string `json:"profile"`
	GrantExpiresAt      string `json:"grantExpiresAt"`
	CredentialExpiresAt string `json:"credentialExpiresAt,omitempty"`
}

// Active holds at most one grant per scope (kube and aws stanzas).
type Active struct {
	Kube *Stanza `json:"kube,omitempty"`
	AWS  *Stanza `json:"aws,omitempty"`
}

// ConfigDir returns ~/.config/zoa/breakglass.
func ConfigDir(home string) string {
	return filepath.Join(home, ".config", "zoa", "breakglass")
}

// ActivePath is the merged active-state file (JSON).
func ActivePath(home string) string {
	return filepath.Join(ConfigDir(home), "active")
}

// AWSCredentialsPath is the dedicated aws-cli credentials file ([default] when aws grant is active).
func AWSCredentialsPath(home string) string {
	return filepath.Join(ConfigDir(home), "aws-credentials")
}

// ScopeForProfile resolves credential scope (kube or aws) from the profile catalog.
func ScopeForProfile(profileID string) (string, error) {
	if def, ok := breakglass.Get(profileID); ok {
		return def.Metadata.Scope, nil
	}
	switch {
	case strings.HasPrefix(profileID, "aws_"):
		return breakglass.ScopeAWS, nil
	case strings.HasPrefix(profileID, "kube_"):
		return breakglass.ScopeKube, nil
	default:
		return "", fmt.Errorf("active: unknown profile %q", profileID)
	}
}

// Read loads active state; missing file yields empty Active.
func Read(home string) (*Active, error) {
	path := ActivePath(home)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Active{}, nil
		}
		return nil, err
	}
	trim := strings.TrimSpace(string(data))
	if trim == "" {
		return &Active{}, nil
	}
	if trim[0] != '{' {
		return nil, fmt.Errorf("active: %s must be JSON (object with kube/aws stanzas)", ActivePath(home))
	}
	var a Active
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// Write persists active state (0600).
func Write(home string, a *Active) error {
	if a == nil {
		a = &Active{}
	}
	dir := ConfigDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(ActivePath(home), data, 0o600)
}

// Activate merges one stanza for the profile's scope (replaces prior grant on that scope).
// At most one active kube and one active aws grant; e.g. kube_write replaces kube_read in active + PS1.
func Activate(home string, stanza Stanza) error {
	scope, err := ScopeForProfile(stanza.Profile)
	if err != nil {
		return err
	}
	a, err := Read(home)
	if err != nil {
		return err
	}
	switch scope {
	case breakglass.ScopeKube:
		a.Kube = &stanza
	case breakglass.ScopeAWS:
		a.AWS = &stanza
	default:
		return fmt.Errorf("active: unsupported scope %q", scope)
	}
	return Write(home, a)
}

// RequestDir is the per-request config directory.
func RequestDir(home, requestID string) string {
	return filepath.Join(ConfigDir(home), requestID)
}
