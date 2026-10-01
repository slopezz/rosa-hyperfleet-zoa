package ecsexec

import (
	"fmt"
	"strings"
)

// ClusterAndTaskFromTaskARN extracts the ECS cluster name and task ID from a task ARN.
// Format: arn:aws:ecs:region:account:task/cluster-name/task-id
func ClusterAndTaskFromTaskARN(taskARN string) (clusterName, taskID string, err error) {
	const prefix = ":task/"
	idx := strings.Index(taskARN, prefix)
	if idx == -1 {
		return "", "", fmt.Errorf("not an ECS task ARN: %q", taskARN)
	}
	rest := taskARN[idx+len(prefix):]
	slash := strings.Index(rest, "/")
	if slash == -1 {
		return "", "", fmt.Errorf("invalid ECS task ARN (missing task id): %q", taskARN)
	}
	return rest[:slash], rest[slash+1:], nil
}

// ClusterNameFromARN returns the resource name from an ECS cluster ARN or plain cluster name.
func ClusterNameFromARN(cluster string) string {
	const marker = ":cluster/"
	if idx := strings.Index(cluster, marker); idx != -1 {
		return cluster[idx+len(marker):]
	}
	return cluster
}
