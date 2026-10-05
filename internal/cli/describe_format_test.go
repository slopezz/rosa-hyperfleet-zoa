package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/cli/parambind"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func catalogActionToClient(name string) *client.Action {
	a, ok := actions.GetCatalog(name)
	if !ok {
		return nil
	}
	m := a.Metadata()
	out := &client.Action{
		Name:                 m.Name,
		Scope:                m.Scope,
		Type:                 m.Type,
		ExecutionMode:        m.ExecutionMode,
		Description:          m.Description,
		Examples:             m.Examples,
		Authorization:        client.ActionAuthorization{Approval: m.Authorization.Approval},
		TimeoutSeconds:       m.TimeoutSeconds,
		WriteCooldownSeconds: m.WriteCooldownSeconds,
		DryRunAction:         m.DryRunAction,
	}
	for _, p := range m.Parameters {
		out.Params = append(out.Params, client.ActionParam{
			Name:        p.Name,
			Description: p.Description,
			Required:    p.Required,
			Default:     p.Default,
		})
	}
	return out
}

func TestPrintDescribeHuman_GetResource_ItShouldMentionGetSecret(t *testing.T) {
	action := catalogActionToClient("get_resource")
	if action == nil {
		t.Fatal("get_resource not in catalog")
	}
	var buf bytes.Buffer
	printDescribeHuman(action, &buf)
	out := buf.String()
	if !containsAll(out, "get_secret", "nodes", "EXAMPLES:") {
		t.Fatalf("unexpected describe output:\n%s", out)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}

// Run with: go test ./internal/cli -run TestDumpDescribeGetResource -v
func TestDumpDescribeGetResource(t *testing.T) {
	if os.Getenv("DUMP_DESCRIBE") != "1" {
		t.Skip("set DUMP_DESCRIBE=1 to print sample describe output")
	}
	action := catalogActionToClient("get_resource")
	var human bytes.Buffer
	printDescribeHuman(action, &human)
	t.Log("=== human ===\n" + human.String())

	view := parambind.EnrichDescribe(action)
	j, _ := json.MarshalIndent(view, "", "  ")
	t.Log("=== json ===\n" + string(j))
}
