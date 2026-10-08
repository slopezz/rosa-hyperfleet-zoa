//go:build e2e

package e2e

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/gomega" //nolint:staticcheck // dot-import is the Gomega/Ginkgo convention
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

// target is one ZOA Lambda deployment (RC or MC) under test. RC and MC are
// separate AWS accounts — each has its own IAM principal allowed to invoke
// its Lambda Function URL, so every subprocess call must be scoped to the
// right AWS_PROFILE for the target it's actually talking to.
type target struct {
	Name             string // human-readable label for Describe()
	DeploymentTarget string // "rc" or "mc" — matches ZOA_DEPLOYMENT_TARGET and gather= values
	APIURL           string
	AWSProfile       string
}

var (
	reasonValue = envOrDefault("E2E_REASON", envOrDefault("E2E_JIRA_TICKET", "ZOAE2E-1"))
	zoaBin      = envOrDefault("ZOA_BIN", "zoa")

	// coredns is the standard EKS system Deployment used for delete_pod and
	// rollout_restart real (non-dry-run) tests. Pods are always owned by a
	// ReplicaSet, so the TA's ownerReferences safety check passes and the
	// controller recreates deleted pods automatically.
	coreDNSNamespace = envOrDefault("E2E_COREDNS_NAMESPACE", "kube-system")
	coreDNSName      = envOrDefault("E2E_COREDNS_NAME", "coredns")
	coreDNSSelector  = envOrDefault("E2E_COREDNS_SELECTOR", "k8s-app=kube-dns")

	targets = discoverTargets()
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// discoverTargets reads ZOA_RC_API_URL / ZOA_MC_API_URL from the environment.
// Either, both, or neither may be set — the suite runs the same spec set
// against whichever targets are present so it can validate a single cluster
// in isolation (e.g. `zoa run` against a dev env) or both at once (CI).
func discoverTargets() []target {
	var out []target
	if url := os.Getenv("ZOA_RC_API_URL"); url != "" {
		out = append(out, target{Name: "RC (Regional Cluster)", DeploymentTarget: "rc", APIURL: url, AWSProfile: envOrDefault("ZOA_RC_AWS_PROFILE", "rrp-rc")})
	}
	if url := os.Getenv("ZOA_MC_API_URL"); url != "" {
		out = append(out, target{Name: "MC (Management Cluster)", DeploymentTarget: "mc", APIURL: url, AWSProfile: envOrDefault("ZOA_MC_AWS_PROFILE", "rrp-mc")})
	}
	return out
}

// runZoa runs the zoa CLI against tgt, scoping ZOA_API_URL/AWS_PROFILE to
// this subprocess's environment only. It deliberately never mutates the
// parent process's environment (os.Setenv) so RC and MC specs can safely run
// in parallel without racing on which profile is "current".
//
// We filter AWS_PROFILE from the inherited environment before setting our own
// to avoid relying on exec.Cmd's "last value wins" behavior for duplicate keys.
func runZoa(tgt target, args ...string) (string, error) {
	cmd := exec.Command(zoaBin, args...) //nolint:gosec // test helper, args are test-controlled
	cmd.Env = append(filterEnv(os.Environ(), "AWS_PROFILE", "ZOA_API_URL"),
		"ZOA_API_URL="+tgt.APIURL,
		"AWS_PROFILE="+tgt.AWSProfile,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// extractJSON extracts the JSON value from CLI output that may contain
// human-readable prefix lines (status indicators like "✓ <id> [cluster]").
// The CLI outputs these even with -o json.
//
// We look for '{' at the start of a line (after newline or at position 0),
// NOT just any '{' or '[' — because status lines contain brackets like
// "[eph-xxx-regional]" which are not JSON.
func extractJSON(output string) string {
	// Look for '{\n' pattern (JSON object starting a line) — this is the most common case
	// Check if output starts with '{'
	if len(output) > 0 && output[0] == '{' {
		return output
	}

	// Look for '\n{' (JSON object after a newline)
	idx := strings.Index(output, "\n{")
	if idx >= 0 {
		return output[idx+1:] // skip the newline, return from '{'
	}

	// Fallback: look for '\n[' for JSON arrays starting a line
	idx = strings.Index(output, "\n[")
	if idx >= 0 {
		return output[idx+1:]
	}

	// Last resort: just find first '{' anywhere (original behavior)
	idx = strings.Index(output, "{")
	if idx >= 0 {
		return output[idx:]
	}

	return output
}

// filterEnv returns a copy of env with the specified keys removed.
func filterEnv(env []string, keys ...string) []string {
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k+"="] = true
	}
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		skip := false
		for prefix := range keySet {
			if strings.HasPrefix(e, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// runActionResult dispatches a Trusted Action via `zoa run` and returns the
// parsed client.Execution JSON (as a generic map, so this suite stays
// decoupled from internal/client's Go types), or an error if the CLI call
// failed, the output wasn't valid JSON, or the execution didn't succeed.
// Unlike runAction, it never asserts — use it from Eventually() polling
// loops where a transient failure (e.g. a Lambda cold start) should be
// retried rather than hard-failing the spec immediately.
func runActionResult(tgt target, action string, extra ...string) (map[string]interface{}, error) {
	args := append([]string{"run", action, "--reason", reasonValue}, extra...)
	args = append(args, "-o", "json")

	out, err := runZoa(tgt, args...)
	if err != nil {
		base := fmt.Errorf("[%s] zoa run %s %v failed: %w:\n%s", tgt.Name, action, extra, err, out)
		return nil, withExecutionLogs(tgt, executionIDFromRunOutput(out), base)
	}

	// CLI outputs human-readable status lines before JSON even with -o json
	jsonStr := extractJSON(out)

	var exec map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(jsonStr), &exec); jsonErr != nil {
		base := fmt.Errorf("[%s] invalid JSON from zoa run %s: %w:\njsonStr=%q\nraw output:\n%s", tgt.Name, action, jsonErr, jsonStr, out)
		return nil, withExecutionLogs(tgt, executionIDFromRunOutput(out), base)
	}
	if exec["status"] != "succeeded" {
		id, _ := exec["id"].(string)
		base := fmt.Errorf("[%s] zoa run %s did not succeed (status=%v):\n%s", tgt.Name, action, exec["status"], out)
		return nil, withExecutionLogs(tgt, id, base)
	}
	return exec, nil
}

// executionIDFromRunOutput extracts an execution UUID from zoa run CLI output
// (JSON body or the human-readable "✓ <id> [cluster]" status line).
func executionIDFromRunOutput(out string) string {
	jsonStr := extractJSON(out)
	var exec struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(jsonStr), &exec) == nil && exec.ID != "" {
		return exec.ID
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "✓ ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			return fields[1]
		}
	}
	return ""
}

// withExecutionLogs appends `zoa logs <id>` output to err when an execution ID
// is known. Best-effort: enrich the original run error with logs when available;
// never replace or hide the underlying runActionResult failure.
func withExecutionLogs(tgt target, executionID string, err error) error {
	if err == nil || executionID == "" {
		return err
	}
	logsOut, logsErr := runZoa(tgt, "logs", executionID)
	if logsErr != nil {
		return fmt.Errorf("%w\n\n(zoa logs %s failed: %v)", err, executionID, logsErr)
	}
	if strings.TrimSpace(logsOut) == "" {
		return fmt.Errorf("%w\n\n--- zoa logs %s ---\n(no logs returned)", err, executionID)
	}
	return fmt.Errorf("%w\n\n--- zoa logs %s ---\n%s", err, executionID, logsOut)
}

// runAction dispatches a Trusted Action expected to succeed and fails the
// calling spec immediately if it didn't. See runActionResult for a
// non-asserting variant.
func runAction(tgt target, action string, extra ...string) map[string]interface{} {
	exec, err := runActionResult(tgt, action, extra...)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return exec
}

// runActionExpectFailure dispatches a Trusted Action expected to fail
// (parameter validation, RBAC denial, HCP namespace protection, ...) and
// returns the combined CLI output for substring assertions on the error.
func runActionExpectFailure(tgt target, action string, extra ...string) string {
	args := append([]string{"run", action, "--reason", reasonValue}, extra...)
	out, err := runZoa(tgt, args...)
	ExpectWithOffset(1, err).To(HaveOccurred(), "zoa run %s %v unexpectedly succeeded:\n%s", action, extra, out)
	return out
}

// outputArray extracts the "output" field of a parsed Execution map as a
// []interface{}, treating a JSON null (server returned zero rows) as an
// empty slice rather than a type-assertion failure.
func outputArray(exec map[string]interface{}) []interface{} {
	v := exec["output"]
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	ExpectWithOffset(1, ok).To(BeTrue(), "expected output to be a JSON array, got %T: %v", v, v)
	return arr
}

// outputMap extracts the "output" field of a parsed Execution map as a
// map[string]interface{}.
func outputMap(exec map[string]interface{}) map[string]interface{} {
	v := exec["output"]
	m, ok := v.(map[string]interface{})
	ExpectWithOffset(1, ok).To(BeTrue(), "expected output to be a JSON object, got %T: %v", v, v)
	return m
}

// eksClusterNamesFromList extracts cluster names from a list_eks_clusters execution.
func eksClusterNamesFromList(exec map[string]interface{}) []string {
	out, ok := exec["output"].(map[string]interface{})
	if !ok {
		return nil
	}
	clusters, ok := out["clusters"].([]interface{})
	if !ok {
		return nil
	}
	names := make([]string, 0, len(clusters))
	for _, c := range clusters {
		if name, ok := c.(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// describeEKSClusterDetailOrNil runs describe_eks_cluster and returns output or nil.
func describeEKSClusterDetailOrNil(tgt target, name string) map[string]interface{} {
	exec, err := runActionResult(tgt, "describe_eks_cluster", "--name", name)
	if err != nil {
		return nil
	}
	detail, ok := exec["output"].(map[string]interface{})
	if !ok {
		return nil
	}
	if detail["name"] != name {
		return nil
	}
	status, _ := detail["status"].(string)
	if status == "" {
		return nil
	}
	return detail
}

// firstDescribableEKSCluster returns the first cluster name and describe output
// the TA can read successfully. E2E_EKS_CLUSTER_NAME pins a specific cluster.
// We do not assert lifecycle status (ACTIVE, etc.) — that is AWS account state
// ZOA does not control; the test only validates the TA round-trip.
func firstDescribableEKSCluster(tgt target) (string, map[string]interface{}) {
	if pinned := os.Getenv("E2E_EKS_CLUSTER_NAME"); pinned != "" {
		if detail := describeEKSClusterDetailOrNil(tgt, pinned); detail != nil {
			return pinned, detail
		}
		return "", nil
	}

	listExec, err := runActionResult(tgt, "list_eks_clusters")
	if err != nil {
		return "", nil
	}
	for _, name := range eksClusterNamesFromList(listExec) {
		if detail := describeEKSClusterDetailOrNil(tgt, name); detail != nil {
			return name, detail
		}
	}
	return "", nil
}

// coreDNSPodNames lists current coredns pod names via get_resource — always
// live state, never an assumed/cached count.
func coreDNSPodNames(tgt target) []string {
	exec := runAction(tgt, "get_resource", "--resource", "pods", "--namespace", coreDNSNamespace, "--selector", coreDNSSelector)
	return podNamesFromOutput(exec["output"])
}

func podNamesFromOutput(output interface{}) []string {
	rows, ok := output.([]interface{})
	if !ok {
		return nil
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		row, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := caseInsensitiveField(row, "name"); ok {
			names = append(names, name)
		}
	}
	return names
}

// caseInsensitiveField looks up key in a row map without hardcoding a
// casing convention: get_resource's rows come straight from the Kubernetes
// server-side Table API (columns like "Name"), which is a K8s API server
// detail, not something ZOA's own code controls.
func caseInsensitiveField(row map[string]interface{}, key string) (string, bool) {
	for k, v := range row {
		if strings.EqualFold(k, key) {
			return fmt.Sprintf("%v", v), true
		}
	}
	return "", false
}

// expectedActionsForDeployment returns TA names that should be registered on a
// Lambda with ZOA_DEPLOYMENT_TARGET=deploymentTarget, derived from pkg/actions metadata
// (same DeploymentTargets filtering the server applies at startup).
func expectedActionsForDeployment(deploymentTarget string) []string {
	var names []string
	for _, a := range actions.ListCatalog() {
		meta := a.Metadata()
		for _, t := range meta.DeploymentTargets {
			if t == deploymentTarget {
				names = append(names, meta.Name)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

// liveActionNames queries a target's Lambda for registered TA names (already
// filtered to that endpoint's deployment target).
func liveActionNames(tgt target) []string {
	out, err := runZoa(tgt, "actions", "-o", "json")
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), out)

	jsonStr := extractJSON(out)
	var list struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	ExpectWithOffset(1, json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)

	names := make([]string, 0, len(list.Items))
	for _, a := range list.Items {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	return names
}

// liveActionScopes returns name→scope for TAs registered on the given endpoint.
func liveActionScopes(tgt target) map[string]string {
	out, err := runZoa(tgt, "actions", "-o", "json")
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), out)

	jsonStr := extractJSON(out)
	var list struct {
		Items []struct {
			Name  string `json:"name"`
			Scope string `json:"scope"`
		} `json:"items"`
	}
	ExpectWithOffset(1, json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)

	scopes := make(map[string]string, len(list.Items))
	for _, a := range list.Items {
		scopes[a.Name] = a.Scope
	}
	return scopes
}

// downloadExecutionOutput saves the execution's primary output artifact via `zoa download`.
func downloadExecutionOutput(tgt target, executionID, destFile string) error {
	out, err := runZoa(tgt, "download", executionID, "-f", destFile)
	if err != nil {
		return fmt.Errorf("[%s] zoa download %s: %w:\n%s", tgt.Name, executionID, err, out)
	}
	return nil
}

// tarballContainsPrefix reports whether any path inside a .tar.gz archive has the
// given prefix (slash-normalized, e.g. "mc/namespaces/kube-applier/").
func tarballContainsPrefix(tarGzPath, prefix string) (bool, error) {
	prefix = filepath.ToSlash(prefix)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	f, err := os.Open(tarGzPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		name := filepath.ToSlash(hdr.Name)
		if strings.HasPrefix(name, prefix) {
			return true, nil
		}
	}
}

// mustGatherPlatformNamespace returns a namespace path marker inside the must_gather
// tarball that is unique to MC vs RC platform dumps.
func mustGatherPlatformNamespace(deploymentTarget string) string {
	switch deploymentTarget {
	case "mc":
		return "mc/namespaces/kube-applier/"
	case "rc":
		return "rc/namespaces/platform-api/"
	default:
		return ""
	}
}

// mustGatherOppositePlatformPrefix returns the top-level gather dir for the other
// deployment (should be absent when gather matches this endpoint only).
func mustGatherOppositePlatformPrefix(deploymentTarget string) string {
	switch deploymentTarget {
	case "mc":
		return "rc/"
	case "rc":
		return "mc/"
	default:
		return ""
	}
}
