package ecsexec

import "testing"

func TestClusterAndTaskFromTaskARN_WhenValidARN_ItShouldParse(t *testing.T) {
	arn := "arn:aws:ecs:us-east-1:599476212575:task/eph-f37869e8-regional-zoa-boundary/0ff74505c2734d9999edf7706c06d455"
	cluster, taskID, err := ClusterAndTaskFromTaskARN(arn)
	if err != nil {
		t.Fatal(err)
	}
	if cluster != "eph-f37869e8-regional-zoa-boundary" {
		t.Errorf("cluster = %q", cluster)
	}
	if taskID != "0ff74505c2734d9999edf7706c06d455" {
		t.Errorf("taskID = %q", taskID)
	}
}

func TestClusterNameFromARN_WhenClusterARN_ItShouldReturnName(t *testing.T) {
	got := ClusterNameFromARN("arn:aws:ecs:us-east-1:599476212575:cluster/eph-f37869e8-regional-zoa-boundary")
	if got != "eph-f37869e8-regional-zoa-boundary" {
		t.Errorf("got %q", got)
	}
}
