package boundaryexec

import "testing"

func TestLogStreamName_WhenBareID_ItShouldAddPrefix(t *testing.T) {
	got := LogStreamName("k7zkjuilu2vhsrp76e48ie3ciy")
	want := "ecs-execute-command-k7zkjuilu2vhsrp76e48ie3ciy"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestValidateExecSessionID_WhenEmpty_ItShouldError(t *testing.T) {
	if err := ValidateExecSessionID(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecLogGroup_WhenTargetCluster_ItShouldUseClusterPath(t *testing.T) {
	got := ExecLogGroup("eph-f37869e8-regional")
	if got != "/ecs/eph-f37869e8-regional/zoa-boundary/ssm-sessions" {
		t.Fatalf("unexpected log group %q", got)
	}
}
