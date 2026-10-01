package accessclient

import "testing"

func TestDefaultExecRoleName_ItShouldMatchHyperfleetDevPattern(t *testing.T) {
	if DefaultExecRoleName != "OrganizationAccountAccessRole" {
		t.Fatalf("unexpected default exec role: %s", DefaultExecRoleName)
	}
}
