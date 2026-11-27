// Package breakglassactivegrants persists which break-glass grants are active on disk:
// ~/.config/zoa/breakglass/active (JSON kube/aws stanzas), per-request kubeconfig dirs,
// and aws-credentials. The shell reads active for PS1/env (11-zoa-breakglass-prompt.bashrc);
// zoa breakglass activate writes via SetGrant.
package breakglassactivegrants

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass"
)

// GrantStanza describes one active grant on a credential scope (kube or aws).
type GrantStanza struct {
	RequestID           string `json:"requestId"`
	Profile             string `json:"profile"`
	GrantExpiresAt      string `json:"grantExpiresAt"`
	CredentialExpiresAt string `json:"credentialExpiresAt,omitempty"`
}

// ActiveFile holds at most one active grant per scope (kube and aws).
type ActiveFile struct {
	Kube *GrantStanza `json:"kube,omitempty"`
	AWS  *GrantStanza `json:"aws,omitempty"`
}

// ConfigDir returns ~/.config/zoa/breakglass.
func ConfigDir(home string) string {
	return filepath.Join(home, ".config", "zoa", "breakglass")
}

// ActiveFilePath is ~/.config/zoa/breakglass/active (shell prompt and env wiring read this file).
func ActiveFilePath(home string) string {
	return filepath.Join(ConfigDir(home), "active")
}

// AWSCredentialsPath is the dedicated aws-cli credentials file ([default] when an aws grant is active).
func AWSCredentialsPath(home string) string {
	return filepath.Join(ConfigDir(home), "aws-credentials")
}

// RequestDir is the per-request config directory under the break-glass config tree.
func RequestDir(home, requestID string) string {
	return filepath.Join(ConfigDir(home), requestID)
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
		return "", fmt.Errorf("breakglassactivegrants: unknown profile %q", profileID)
	}
}

// Read loads the active-grants file; a missing file yields an empty ActiveFile.
func Read(home string) (*ActiveFile, error) {
	path := ActiveFilePath(home)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ActiveFile{}, nil
		}
		return nil, err
	}
	trim := strings.TrimSpace(string(data))
	if trim == "" {
		return &ActiveFile{}, nil
	}
	if trim[0] != '{' {
		return nil, fmt.Errorf("breakglassactivegrants: %s must be JSON (object with kube/aws stanzas)", ActiveFilePath(home))
	}
	var f ActiveFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Write persists the active-grants file (0600).
func Write(home string, f *ActiveFile) error {
	if f == nil {
		f = &ActiveFile{}
	}
	dir := ConfigDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(ActiveFilePath(home), data, 0o600)
}

// SetGrant merges one stanza for the profile's scope (replaces any prior grant on that scope).
func SetGrant(home string, stanza GrantStanza) error {
	scope, err := ScopeForProfile(stanza.Profile)
	if err != nil {
		return err
	}
	f, err := Read(home)
	if err != nil {
		return err
	}
	switch scope {
	case breakglass.ScopeKube:
		f.Kube = &stanza
	case breakglass.ScopeAWS:
		f.AWS = &stanza
	default:
		return fmt.Errorf("breakglassactivegrants: unsupported scope %q", scope)
	}
	return Write(home, f)
}
