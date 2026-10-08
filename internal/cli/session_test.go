package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func TestFormatExecSessionsForTable(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{
			name: "When no exec sessions it should dash",
			ids:  nil,
			want: "-",
		},
		{
			name: "When one exec session it should show short id",
			ids:  []string{"ecs-execute-command-k7zkjuilu2vhsrp76e48ie3ciy"},
			want: "k7zkjuilu2vhsrp76e48ie3ciy",
		},
		{
			name: "When multiple exec sessions it should show join count",
			ids:  []string{"ecs-execute-command-a", "ecs-execute-command-b"},
			want: "2 joins",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExecSessionsForTable(tt.ids)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintSessionTable_WhenListMode_ItShouldIncludeStopReasonAndEnded(t *testing.T) {
	list := &client.SessionList{
		Items: []client.Session{
			{
				SessionID:      "7840ac64-65f7-4198-b9da-8e38fc985d42",
				TargetCluster:  "eph-regional",
				Reason:         "ROSAENG-1234",
				Status:         "terminated",
				StopReason:     "idleReaperStop",
				ExecSessionIDs: []string{"ecs-execute-command-k7zkjuilu2vhsrp76e48ie3ciy"},
				CreatedAt:      "2026-10-05T19:46:42.79550334Z",
				CompletedAt:    "2026-10-05T21:24:40.967512578Z",
				Deadline:       "2026-10-05T23:46:42.79550334Z",
			},
		},
		Count: 1,
	}

	var buf bytes.Buffer
	opts := &GlobalOptions{OutputFormat: output.FormatTable}
	if err := printSessionTable(&buf, opts, "us-east-1-eph", list, false); err != nil {
		t.Fatalf("printSessionTable: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "REASON") || !strings.Contains(out, "STOP REASON") || !strings.Contains(out, "ENDED") {
		t.Fatalf("expected REASON, STOP REASON and ENDED columns, got:\n%s", out)
	}
	if !strings.Contains(out, "ROSAENG-1234") {
		t.Fatalf("expected audit reason in output, got:\n%s", out)
	}
	if !strings.Contains(out, "EXEC SESSIONS") {
		t.Fatalf("expected EXEC SESSIONS column, got:\n%s", out)
	}
	if !strings.Contains(out, "idleReaperStop") {
		t.Fatalf("expected stop reason in output, got:\n%s", out)
	}
	if !strings.Contains(out, "k7zkjuilu2vhsrp76e48ie3ciy") {
		t.Fatalf("expected exec session id in output, got:\n%s", out)
	}
	if !strings.Contains(out, "2026-10-05T21:24:40.967512578Z") {
		t.Fatalf("expected ended timestamp in output, got:\n%s", out)
	}
}

func TestPrintSessionTable_WhenWide_ItShouldIncludeIdentityColumns(t *testing.T) {
	signer := "arn:aws:sts::123456:assumed-role/sre-role/slopezma"
	list := &client.SessionList{
		Items: []client.Session{
			{
				SessionID:     "7840ac64-65f7-4198-b9da-8e38fc985d42",
				Operator:      "slopezma",
				SignerARN:     signer,
				AccountID:     "123456789012",
				TaskID:        "6e8699e3938a4bcd1234567890abcdef",
				TargetCluster: "eph-regional",
				Status:        "active",
			},
		},
		Count: 1,
	}

	var buf bytes.Buffer
	opts := &GlobalOptions{OutputFormat: output.FormatWide}
	if err := printSessionTable(&buf, opts, "us-east-1-eph", list, true); err != nil {
		t.Fatalf("printSessionTable: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"SIGNER_ARN", "ACCOUNT_ID", "TASK_ID", signer, "123456789012"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestResolveDeploymentTarget(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		flagDeployment string
		flagTarget     string
		wantDeployment string
		wantTarget     string
	}{
		{
			name:           "When both positional args are given it should use them",
			args:           []string{"us-east-1", "mc01"},
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When positional args are given they should override flags",
			args:           []string{"us-east-1", "mc01"},
			flagDeployment: "eu-west-1",
			flagTarget:     "mc02",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When one positional arg is given it should be deployment with flag target",
			args:           []string{"us-east-1"},
			flagTarget:     "mc01",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When no positional args are given it should fall back to flags",
			args:           []string{},
			flagDeployment: "us-east-1",
			flagTarget:     "mc01",
			wantDeployment: "us-east-1",
			wantTarget:     "mc01",
		},
		{
			name:           "When nothing is provided it should return empty strings",
			args:           []string{},
			wantDeployment: "",
			wantTarget:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment, target := resolveDeploymentTarget(tt.args, tt.flagDeployment, tt.flagTarget)
			if deployment != tt.wantDeployment {
				t.Errorf("deployment = %q, want %q", deployment, tt.wantDeployment)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
		})
	}
}
