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

// Stanza is one active grant on a plane (kube or aws).
type Stanza struct {
	RequestID         string `json:"requestId"`
	Scope             string `json:"scope"`
	GrantExpiresAt    string `json:"grantExpiresAt"`
	BearerExpiresAt   string `json:"bearerExpiresAt,omitempty"`
}

// Active holds at most one grant per plane.
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

// PlaneForScope resolves kube vs aws from the scope catalog.
func PlaneForScope(scopeID string) (string, error) {
	if def, ok := breakglass.Get(scopeID); ok {
		return def.Metadata.Plane, nil
	}
	switch {
	case strings.HasPrefix(scopeID, "aws-"):
		return breakglass.PlaneAWS, nil
	case strings.HasPrefix(scopeID, "kube-"):
		return breakglass.PlaneKube, nil
	default:
		return "", fmt.Errorf("active: unknown scope %q", scopeID)
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
		return migrateLegacy(home, trim)
	}
	var a Active
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func migrateLegacy(home, requestID string) (*Active, error) {
	requestID = strings.TrimSpace(requestID)
	metaPath := filepath.Join(ConfigDir(home), requestID, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return &Active{}, nil
	}
	var meta struct {
		Scope          string `json:"scope"`
		GrantExpiresAt string `json:"grant_expires_at"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return &Active{}, nil
	}
	plane, err := PlaneForScope(meta.Scope)
	if err != nil {
		return &Active{}, nil
	}
	a := &Active{}
	stanza := Stanza{RequestID: requestID, Scope: meta.Scope, GrantExpiresAt: meta.GrantExpiresAt}
	switch plane {
	case breakglass.PlaneKube:
		a.Kube = &stanza
	case breakglass.PlaneAWS:
		a.AWS = &stanza
	}
	_ = Write(home, a)
	return a, nil
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

// Activate merges one stanza for the scope's plane (replaces prior grant on that plane).
// At most one active kube and one active aws grant; e.g. kube-write replaces kube-read in active + PS1.
func Activate(home string, stanza Stanza) error {
	plane, err := PlaneForScope(stanza.Scope)
	if err != nil {
		return err
	}
	a, err := Read(home)
	if err != nil {
		return err
	}
	switch plane {
	case breakglass.PlaneKube:
		a.Kube = &stanza
	case breakglass.PlaneAWS:
		a.AWS = &stanza
	default:
		return fmt.Errorf("active: unsupported plane %q", plane)
	}
	return Write(home, a)
}

// RequestDir is the per-request config directory.
func RequestDir(home, requestID string) string {
	return filepath.Join(ConfigDir(home), requestID)
}
