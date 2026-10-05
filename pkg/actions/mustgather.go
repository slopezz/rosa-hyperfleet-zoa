package actions

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/labels"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	mustGatherImage         = "quay.io/stolostron/must-gather:5.1.0-SNAPSHOT-2026-09-10-00-23-02"
	mustGatherOutputDir     = "/must-gather"
	mustGatherInitContainer = "must-gather"
	mustGatherHoldContainer = "hold"
	mustGatherHoldSleepSecs = 3600
	mustGatherTimeout       = 1800 // 30 minutes — async Job deadline; tune down after MC measurements
	podWaitInterval         = 5 * time.Second
	directDumpReserve       = 3 * time.Minute // reserved for direct dump + tarball + upload
	execCopyTimeout         = 120 * time.Second
)

type mustGatherArtifacts struct {
	Tarball      string
	TarballBytes int64
}

const (
	mustGatherPodComponentLabel = labels.ComponentMustGather
)

// gatherTarget selects which diagnostic profiles to run on the ZOA-connected EKS cluster.
var validGatherTargets = map[string]struct{}{
	"hcp": {},
	"mc":  {},
	"rc":  {},
}

// hcpSupplementPlatformNamespaces are MC platform namespaces relevant to HCP
// diagnostics that the gather image (hypershift dump cluster) doesn't cover.
// Collected under hcp/ when gather=hcp without mc; when mc is also requested
// these namespaces are already covered by collectDirectDiagnostics in the mc branch.
var hcpSupplementPlatformNamespaces = []string{
	"hypershift",
	"cert-manager",
	"kube-applier",
}

// mcPlatformNamespaces are management-cluster platform namespaces included in direct dump.
// Derived from rosa-hyperfleet argocd/config/{management-cluster,shared}/* ApplicationSet destinations.
var mcPlatformNamespaces = []string{
	"kube-system",
	"argocd",
	"eks-nodepool",
	"external-secrets",
	"karpenter",
	"cert-manager",
	"cloudwatch-exporter",
	"external-secrets-config",
	"hypershift",
	"kube-applier",
	"monitoring",
	"vector",
}

// rcPlatformNamespaces are regional-cluster platform namespaces included in direct dump.
// Derived from rosa-hyperfleet argocd/config/{regional-cluster,shared}/* ApplicationSet destinations.
var rcPlatformNamespaces = []string{
	"kube-system",
	"argocd",
	"eks-nodepool",
	"external-secrets",
	"karpenter",
	"aws-load-balancer-controller",
	"cloudwatch-exporter",
	"external-secrets-config",
	"grafana",
	"hyperfleet",
	"loki",
	"monitoring",
	"platform-api",
	"thanos",
	"thanos-operator",
	"vector",
}

type clusterIdentity struct {
	ClusterID         string
	HCNamespace       string
	HostedClusterName string
	CPNamespace       string
}

func init() {
	Register(&mustGather{})
}

type mustGather struct{}

func (m *mustGather) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:                          "must_gather",
		Scope:                         "kube-api",
		Type:                          "read",
		ExecutionMode:                 "async",
		DisallowExecutionModeOverride: true,
		Description:                   "Collect must-gather bundle (logs, events, resources) for hosted cluster, MC platform, or RC platform. MC ZOA: gather mc|hcp (hcp needs cluster-id). RC ZOA: gather rc.",
		Authorization:                 AuthorizationConfig{Approval: "none"},
		TimeoutSeconds:                mustGatherTimeout,
		DeploymentTargets:             []string{DeploymentTargetRC, DeploymentTargetMC},
		Examples: []string{
			"zoa run must_gather --gather mc --jira ROSAENG-1234 --wait",
			"zoa run must_gather --gather hcp --cluster-id 01234567-89ab-cdef-0123-456789abcdef --jira ROSAENG-1234 --wait",
			"zoa run must_gather --gather rc --jira ROSAENG-1234 --wait",
		},
		Parameters: []ParameterDef{
			{Name: "gather", Required: true, Description: "Scope: hcp (hosted cluster + control plane), mc (management platform namespaces), rc (regional platform + HyperFleet CRs). Comma-separated; must match this ZOA endpoint."},
			{Name: "cluster_id", Description: "Hosted cluster UUID. Required when gather includes hcp; ignored for mc/rc-only gathers."},
			{Name: "extra_namespaces", Description: "Optional comma-separated namespaces added to the dump for the selected scope(s)."},
			{Name: "skip_must_gather_image", Default: "false", Description: "When gather includes hcp: skip hypershift dump pod and collect Kubernetes log supplement only."},
		},
		RBAC: &RBACConfig{
			ClusterScoped:   true,
			AllowSecretRead: true,
			Rules: []RBACRule{
				// Diagnostic read for hypershift dump / oc adm inspect (release-agnostic; same pattern as get_resource).
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get", "list"}},
				// Operational verbs not covered by read-only wildcard.
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create", "delete"}},
				{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"get"}},
				{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
				{APIGroups: []string{""}, Resources: []string{"pods/portforward"}, Verbs: []string{"create"}},
			},
		},
	}
}

func (m *mustGather) Validate(ctx context.Context, params *ExecutionParams) error {
	if strings.TrimSpace(params.Params["gather"]) == "" {
		return fmt.Errorf("parameter %q is required", "gather")
	}

	targets, err := parseGatherTargets(params.Params["gather"])
	if err != nil {
		return err
	}

	deploymentTarget := params.DeploymentTarget
	if deploymentTarget == "" {
		deploymentTarget = DeploymentTarget()
	}
	if err := validateGatherForDeployment(deploymentTarget, targets); err != nil {
		return err
	}

	if gatherIncludes(targets, "hcp") {
		if normalizeClusterID(params.Params["cluster_id"]) == "" {
			return fmt.Errorf("cluster_id is required when gather includes hcp")
		}
		if _, err := resolveClusterIdentity(ctx, params); err != nil {
			return err
		}
	}

	if params.KubeClient == nil {
		return fmt.Errorf("kubernetes client is required")
	}
	if gatherIncludes(targets, "hcp") && !isSkipMustGatherImage(params.Params) && params.RESTConfig == nil {
		return fmt.Errorf("REST config is required for pod exec")
	}

	return nil
}

func isSkipMustGatherImage(params map[string]string) bool {
	return params["skip_must_gather_image"] == "true"
}

func (m *mustGather) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	targets, err := parseGatherTargets(params.Params["gather"])
	if err != nil {
		return nil, err
	}

	var id clusterIdentity
	if gatherIncludes(targets, "hcp") {
		id, err = resolveClusterIdentity(ctx, params)
		if err != nil {
			return nil, err
		}
	}

	skipImage := isSkipMustGatherImage(params.Params)
	extraNamespaces := parseNamespaceList(params.Params["extra_namespaces"])

	params.Logger.Info("starting must-gather collection",
		"gather", strings.Join(targets, ","),
		"cluster_id", id.ClusterID,
		"namespace", id.HCNamespace,
		"hosted_cluster", id.HostedClusterName,
		"skip_must_gather_image", params.Params["skip_must_gather_image"],
		"run_gather_image", gatherIncludes(targets, "hcp") && !skipImage,
	)

	outputDir := "/output"
	if envDir := os.Getenv("ZOA_OUTPUT_DIR"); envDir != "" {
		outputDir = envDir
	}

	var gatherImageErr error
	runGatherImage := gatherIncludes(targets, "hcp") && !skipImage
	if gatherIncludes(targets, "hcp") {
		hcpDir := filepath.Join(outputDir, "hcp")
		if err := os.MkdirAll(hcpDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating hcp gather directory: %w", err)
		}
		if runGatherImage {
			gatherImageErr = m.runMustGatherPod(ctx, params, id.HCNamespace, id.HostedClusterName, hcpDir)
			if gatherImageErr != nil {
				params.Logger.Warn("gather image collection failed", "error", gatherImageErr)
			}
		}

		// Direct diagnostics for HCP-relevant namespaces not covered by the gather image.
		// Platform namespaces (hypershift, cert-manager, kube-applier) are skipped when mc
		// is also requested — the mc branch covers them via mcPlatformNamespaces.
		var hcpDirectNS []string
		if !gatherIncludes(targets, "mc") {
			hcpDirectNS = append(hcpDirectNS, hcpSupplementPlatformNamespaces...)
		}
		// When the image was skipped or failed, also collect CPNamespace and HCNamespace
		// directly (the image normally provides logs for these via hypershift dump cluster).
		if skipImage || gatherImageErr != nil {
			if id.CPNamespace != "" {
				hcpDirectNS = append(hcpDirectNS, id.CPNamespace)
			}
			if id.HCNamespace != "" {
				hcpDirectNS = append(hcpDirectNS, id.HCNamespace)
			}
		}
		hcpDirectNS = mappendUniqueNamespaces(hcpDirectNS, extraNamespaces)
		if len(hcpDirectNS) > 0 {
			m.collectDirectDiagnostics(ctx, params, hcpDirectNS, hcpDir)
		}
	}

	targetNamespaces := m.resolveCollectionNamespaces(targets, id, extraNamespaces)

	if gatherIncludes(targets, "mc") {
		mcDir := filepath.Join(outputDir, "mc")
		if err := os.MkdirAll(mcDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating mc gather directory: %w", err)
		}
		mcNamespaces := m.resolveCollectionNamespaces([]string{"mc"}, id, extraNamespaces)
		m.collectDirectDiagnostics(ctx, params, mcNamespaces, mcDir)
		m.collectClusterNodes(ctx, params, mcDir)
		m.collectClusterStorage(ctx, params, mcDir)
		m.collectKarpenterCRs(ctx, params, mcDir)
	}
	if gatherIncludes(targets, "rc") {
		rcDir := filepath.Join(outputDir, "rc")
		if err := os.MkdirAll(rcDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating rc gather directory: %w", err)
		}
		rcNamespaces := m.resolveCollectionNamespaces([]string{"rc"}, id, extraNamespaces)
		m.collectDirectDiagnostics(ctx, params, rcNamespaces, rcDir)
		m.collectRegionalCRs(ctx, params, rcDir)
		m.collectClusterNodes(ctx, params, rcDir)
		m.collectClusterStorage(ctx, params, rcDir)
		m.collectKarpenterCRs(ctx, params, rcDir)
	}

	// Package into output.tar.gz — top-level hcp/, mc/, rc/ dirs match osdctl layout for omc/omg.
	tarballPath := filepath.Join(outputDir, "output.tar.gz")
	tarInfo, err := createTarball(outputDir, tarballPath)
	if err != nil {
		return nil, fmt.Errorf("creating tarball: %w", err)
	}

	summary := buildMustGatherSummary(id, targets, skipImage, gatherImageErr)
	logMustGatherReport(params.Logger, targets, skipImage, gatherImageErr, targetNamespaces, tarInfo)

	success := mustGatherExecutionSuccess(targets, skipImage, gatherImageErr)
	if !success {
		params.Logger.Error("must-gather completed with errors", "summary", summary)
	}

	// Return nil Output so the runner skips writing output.json, allowing
	// ArtifactSizes() to detect output.tar.gz as the primary artifact.
	// Collection details are recorded in execution.log (must_gather_report event).
	// Partial tarball is still uploaded when success is false (runner exit 1).
	return &ActionResult{
		Success: success,
		Output:  nil,
		Summary: summary,
	}, nil
}

// mustGatherExecutionSuccess is false when hcp gather image was required but failed.
// Direct dump may still have produced a partial tarball for download.
func mustGatherExecutionSuccess(targets []string, skipImage bool, gatherImageErr error) bool {
	if gatherImageErr == nil {
		return true
	}
	if skipImage || !gatherIncludes(targets, "hcp") {
		return true
	}
	return false
}

// runMustGatherPod creates and manages the must-gather child pod.
func (m *mustGather) runMustGatherPod(ctx context.Context, params *ExecutionParams, hcpNamespace, hostedClusterName, gatherDir string) error {
	podName := mustGatherPodName(params.ExecutionID)
	namespace := labels.JobsNamespace()

	saName := fmt.Sprintf("zoa-exec-%s", params.ExecutionID)
	hostedClusterNS := hcNamespaceFromCPNamespace(hcpNamespace, hostedClusterName)
	params.Logger.Info("creating must-gather pod",
		"pod", podName,
		"namespace", namespace,
		"execution_id", params.ExecutionID,
		"service_account", saName,
		"hosted_cluster_namespace", hostedClusterNS,
		"hosted_cluster_name", hostedClusterName,
	)

	// HyperShift dump via gather_utils (ACM /usr/bin/gather skips dump on bare hosting MCs).
	gatherCmd := mustGatherHypershiftDumpScript(hostedClusterNS, hostedClusterName)

	gatherVolumeMount := corev1.VolumeMount{Name: "gather-output", MountPath: mustGatherOutputDir}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				labels.KeyManagedBy:   labels.ValueManagedByZOA,
				labels.KeyComponent:   mustGatherPodComponentLabel,
				labels.KeyExecutionID: truncateID(params.ExecutionID, 63),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: saName,
			// Gather runs in init so the hold sidecar keeps a live container for exec copy.
			InitContainers: []corev1.Container{
				{
					Name:         mustGatherInitContainer,
					Image:        mustGatherImage,
					Command:      []string{"/bin/sh", "-c", gatherCmd},
					VolumeMounts: []corev1.VolumeMount{gatherVolumeMount},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    mustGatherHoldContainer,
					Image:   mustGatherImage,
					Command: []string{"sleep", fmt.Sprintf("%d", mustGatherHoldSleepSecs)},
					VolumeMounts: []corev1.VolumeMount{
						gatherVolumeMount,
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "gather-output",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	// Idempotent retry for the same execution (Job restart): remove our pod only.
	_ = params.KubeClient.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})

	_, err := params.KubeClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating must-gather pod: %w", err)
	}

	defer func() {
		params.Logger.Info("cleaning up must-gather pod")
		_ = params.KubeClient.CoreV1().Pods(namespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
	}()

	// Wait for gather init container success while hold sidecar is still running for exec copy.
	waitErr := m.waitForGatherReadyToCopy(ctx, params, namespace, podName)
	m.logGatherPodInitContainer(ctx, params, namespace, podName)
	if waitErr != nil {
		return fmt.Errorf("waiting for gather completion: %w", waitErr)
	}

	// Copy output from the hold sidecar (gather init container has already exited).
	if err := m.copyFromPod(ctx, params, namespace, podName, mustGatherOutputDir, gatherDir); err != nil {
		return fmt.Errorf("copying must-gather output: %w", err)
	}
	if err := validateHCPDumpOutput(gatherDir); err != nil {
		return err
	}

	return nil
}

// mustGatherHypershiftDumpScript runs hypershift dump cluster from the must-gather image.
// Uses gather_utils for HC arg parsing and CLI discovery; skips inner hypershift-dump.tar.gz
// (--archive-dump=false) since ZOA wraps hcp/ in output.tar.gz.
func mustGatherHypershiftDumpScript(hostedClusterNS, hostedClusterName string) string {
	return fmt.Sprintf(`set -e
export BASE_COLLECTION_PATH=%q
. /usr/bin/gather_utils
parse_args hosted-cluster-namespace=%s hosted-cluster-name=%s
ensure_hypershift_cli || exit 1
hypershift_cmd=("${HYPERSHIFT}" dump cluster --artifact-dir "${BASE_COLLECTION_PATH}" --name "${HC_NAME}" --namespace "${HC_NAMESPACE}" --archive-dump=false)
if ! "${hypershift_cmd[@]}" --dump-guest-cluster=fail-on-error; then
  if ! "${hypershift_cmd[@]}" --dump-guest-cluster=fail-on-error,direct-kube-api-service-access; then
    exit 1
  fi
fi
`, mustGatherOutputDir, hostedClusterNS, hostedClusterName)
}

// validateHCPDumpOutput rejects empty hypershift dumps (ACM gather used to exit 0 with no files).
func validateHCPDumpOutput(dir string) error {
	markers := []string{
		"cluster-scoped-resources",
		"namespaces",
		"hypershift-dump.tar.gz", // legacy inner archive from older runs
	}
	for _, name := range markers {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return nil
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading hypershift dump output: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "hostedcluster-") {
			return nil
		}
	}
	if len(entries) == 0 {
		return fmt.Errorf("hypershift dump produced no output under %s", dir)
	}
	return fmt.Errorf("hypershift dump output missing expected artifacts under %s", dir)
}

func (m *mustGather) waitForGatherReadyToCopy(ctx context.Context, params *ExecutionParams, namespace, podName string) error {
	params.Logger.Info("waiting for must-gather init container to complete")

	waitTimeout := pollTimeoutFromContext(ctx, time.Duration(mustGatherTimeout)*time.Second-directDumpReserve)

	return wait.PollUntilContextTimeout(ctx, podWaitInterval, waitTimeout, true, func(ctx context.Context) (bool, error) {
		pod, err := params.KubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("getting pod status: %w", err)
		}

		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("%s", mustGatherPodFailureReason(pod))
		}

		if err := mustGatherInitContainerFailure(pod); err != nil {
			return false, err
		}
		if !mustGatherInitContainerSucceeded(pod) {
			return false, nil
		}

		if !containerRunning(pod, mustGatherHoldContainer) {
			return false, nil
		}

		params.Logger.Info("must-gather init container completed successfully")
		return true, nil
	})
}

func mustGatherInitContainerSucceeded(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name != mustGatherInitContainer {
			continue
		}
		return cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0
	}
	return false
}

func mustGatherInitContainerFailure(pod *corev1.Pod) error {
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name != mustGatherInitContainer {
			continue
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return fmt.Errorf("%s", containerFailureReason(mustGatherInitContainer, cs.State.Terminated))
		}
	}
	return nil
}

func containerRunning(pod *corev1.Pod, name string) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == name && cs.State.Running != nil {
			return true
		}
	}
	return false
}

func containerFailureReason(container string, state *corev1.ContainerStateTerminated) string {
	reason := state.Reason
	if reason == "" {
		reason = "Error"
	}
	if state.Message != "" {
		return fmt.Sprintf("must-gather pod failed: container %s exit %d (%s): %s",
			container, state.ExitCode, reason, state.Message)
	}
	return fmt.Sprintf("must-gather pod failed: container %s exit %d (%s)",
		container, state.ExitCode, reason)
}

func mustGatherPodFailureReason(pod *corev1.Pod) string {
	if err := mustGatherInitContainerFailure(pod); err != nil {
		return err.Error()
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return containerFailureReason(cs.Name, cs.State.Terminated)
		}
	}
	return "must-gather pod failed"
}

// copyFromPod copies files from a pod container to local filesystem using tar exec.
func (m *mustGather) copyFromPod(ctx context.Context, params *ExecutionParams, namespace, podName, srcPath, dstPath string) error {
	params.Logger.Info("copying data from must-gather pod", "src", srcPath, "dst", dstPath)

	tarCmd := []string{"tar", "czf", "-", "-C", srcPath, "."}

	// Stream the tarball directly from the pod to disk via io.Pipe to avoid
	// buffering the entire archive in memory (can exceed Lambda 512MB limit).
	pr, pw := io.Pipe()

	req := params.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: mustGatherHoldContainer,
			Command:   tarCmd,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(params.RESTConfig, "POST", req.URL())
	if err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("creating SPDY executor: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, execCopyTimeout)

	var execErr error
	var stderr bytes.Buffer
	go func() {
		defer cancel()
		defer pw.Close()
		execErr = exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
			Stdout: pw,
			Stderr: &stderr,
		})
		if execErr != nil {
			pw.CloseWithError(fmt.Errorf("stream exec: %w (stderr: %s)", execErr, stderr.String()))
		}
	}()

	extractErr := extractTarGz(pr, dstPath)
	pr.Close()

	if execErr != nil {
		return fmt.Errorf("exec tar in pod: %w (stderr: %s)", execErr, stderr.String())
	}
	return extractErr
}

func parseGatherTargets(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("parameter %q is required", "gather")
	}

	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	var targets []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		if _, ok := validGatherTargets[p]; !ok {
			return nil, fmt.Errorf("invalid gather target %q (allowed: hcp, mc, rc)", p)
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		targets = append(targets, p)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("parameter %q is required", "gather")
	}
	return targets, nil
}

func validateGatherForDeployment(deploymentTarget string, targets []string) error {
	if deploymentTarget == "" {
		return nil
	}
	for _, t := range targets {
		switch deploymentTarget {
		case "rc":
			if t != "rc" {
				return fmt.Errorf("gather %q is not allowed on rc ZOA endpoint (allowed: rc)", t)
			}
		case "mc":
			if t == "rc" {
				return fmt.Errorf("gather %q is not allowed on mc ZOA endpoint (allowed: hcp, mc)", t)
			}
		default:
			return fmt.Errorf("invalid deployment target %q", deploymentTarget)
		}
	}
	return nil
}

func gatherIncludes(targets []string, name string) bool {
	for _, t := range targets {
		if t == name {
			return true
		}
	}
	return false
}

func normalizeClusterID(id string) string {
	return strings.TrimPrefix(strings.TrimSpace(id), labels.ClusterNamespacePrefix)
}

func clusterNamespaceFromID(id string) string {
	norm := normalizeClusterID(id)
	if norm == "" {
		return ""
	}
	return labels.ClusterNamespacePrefix + norm
}

func resolveClusterIdentity(ctx context.Context, params *ExecutionParams) (clusterIdentity, error) {
	var id clusterIdentity

	clusterID := normalizeClusterID(params.Params["cluster_id"])
	if clusterID == "" {
		return id, fmt.Errorf("cluster_id is required when gather includes hcp")
	}

	id.ClusterID = clusterID
	id.HCNamespace = clusterNamespaceFromID(clusterID)

	name, err := lookupHostedClusterName(ctx, params, id.HCNamespace)
	if err != nil {
		return id, err
	}
	id.HostedClusterName = name
	id.CPNamespace = fmt.Sprintf("%s-%s", id.HCNamespace, id.HostedClusterName)
	return id, nil
}

func lookupHostedClusterName(ctx context.Context, params *ExecutionParams, hcNamespace string) (string, error) {
	if params.DynamicClient == nil {
		return "", fmt.Errorf("cannot resolve HostedCluster in %q: dynamic client unavailable", hcNamespace)
	}

	gvr := schema.GroupVersionResource{
		Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedclusters",
	}
	list, err := params.DynamicClient.Resource(gvr).Namespace(hcNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing HostedClusters in %q: %w", hcNamespace, err)
	}
	switch len(list.Items) {
	case 0:
		return "", fmt.Errorf("no HostedCluster found in namespace %q", hcNamespace)
	case 1:
		return list.Items[0].GetName(), nil
	default:
		names := make([]string, len(list.Items))
		for i := range list.Items {
			names[i] = list.Items[i].GetName()
		}
		return "", fmt.Errorf("multiple HostedClusters in %q (%s); expected exactly one", hcNamespace, strings.Join(names, ", "))
	}
}

func (m *mustGather) resolveCollectionNamespaces(targets []string, id clusterIdentity, extra []string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(ns string) {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			return
		}
		if _, ok := seen[ns]; ok {
			return
		}
		seen[ns] = struct{}{}
		out = append(out, ns)
	}

	if gatherIncludes(targets, "hcp") && id.HCNamespace != "" {
		add(id.HCNamespace)
		add(id.CPNamespace)
		for _, ns := range hcpSupplementPlatformNamespaces {
			add(ns)
		}
	}
	if gatherIncludes(targets, "mc") {
		for _, ns := range mcPlatformNamespaces {
			add(ns)
		}
	}
	if gatherIncludes(targets, "rc") {
		for _, ns := range rcPlatformNamespaces {
			add(ns)
		}
	}
	for _, ns := range extra {
		add(ns)
	}
	return out
}

// mappendUniqueNamespaces deduplicates and merges extra namespaces into base.
func mappendUniqueNamespaces(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	var out []string
	add := func(ns string) {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			return
		}
		if _, ok := seen[ns]; ok {
			return
		}
		seen[ns] = struct{}{}
		out = append(out, ns)
	}
	for _, ns := range base {
		add(ns)
	}
	for _, ns := range extra {
		add(ns)
	}
	return out
}

func (m *mustGather) logGatherPodInitContainer(ctx context.Context, params *ExecutionParams, namespace, podName string) {
	logText, fetchErr := m.fetchPodContainerLogText(ctx, params, namespace, podName, mustGatherInitContainer)
	issueCount, samples := scanGatherLogIssues(logText)
	attrs := []any{
		"event", "gather_pod_init_log",
		"container", mustGatherInitContainer,
		"bytes", len(logText),
		"log", logText,
		"issue_count", issueCount,
	}
	if len(samples) > 0 {
		attrs = append(attrs, "issue_samples", samples)
	}
	if fetchErr != nil {
		attrs = append(attrs, "fetch_error", fetchErr.Error())
	}
	params.Logger.Info("must-gather pod init container log", attrs...)
}

func (m *mustGather) fetchPodContainerLogText(ctx context.Context, params *ExecutionParams, namespace, podName, container string) (string, error) {
	opts := &corev1.PodLogOptions{Container: container}
	stream, err := params.KubeClient.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	data, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

var gatherLogIssueSubstrings = []string{
	"forbidden",
	"unauthorized",
	"cannot list",
	"failed to get",
	"access denied",
	"permission denied",
}

func scanGatherLogIssues(log string) (count int, samples []string) {
	if log == "" {
		return 0, nil
	}
	for _, line := range strings.Split(log, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		for _, needle := range gatherLogIssueSubstrings {
			if strings.Contains(lower, needle) {
				count++
				if len(samples) < 10 {
					samples = append(samples, trimmed)
				}
				break
			}
		}
	}
	return count, samples
}

func (m *mustGather) collectClusterNodes(ctx context.Context, params *ExecutionParams, gatherDir string) {
	nodes, err := params.KubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list cluster nodes", "error", err)
		return
	}
	dir := filepath.Join(gatherDir, "cluster-scoped-resources", "core")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		params.Logger.Warn("failed to create nodes dir", "error", err)
		return
	}
	data, err := json.MarshalIndent(nodes.Items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "nodes.json"), data, 0o644)
}

func (m *mustGather) collectClusterStorage(ctx context.Context, params *ExecutionParams, gatherDir string) {
	storageClasses, err := params.KubeClient.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list storage classes", "error", err)
	} else {
		dir := filepath.Join(gatherDir, "cluster-scoped-resources", "storage.k8s.io")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			data, err := json.MarshalIndent(storageClasses.Items, "", "  ")
			if err == nil {
				_ = os.WriteFile(filepath.Join(dir, "storageclasses.json"), data, 0o644)
			}
		}
	}

	pvs, err := params.KubeClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list persistent volumes", "error", err)
		return
	}
	dir2 := filepath.Join(gatherDir, "cluster-scoped-resources", "core")
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		params.Logger.Warn("failed to create persistent volumes dir", "error", err)
		return
	}
	data2, err := json.MarshalIndent(pvs.Items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir2, "persistentvolumes.json"), data2, 0o644)
}

func (m *mustGather) collectNamespacedDynamicResources(ctx context.Context, params *ExecutionParams, namespace, subdir string, gvrs []schema.GroupVersionResource, gatherDir string) {
	if params.DynamicClient == nil || namespace == "" {
		return
	}
	base := filepath.Join(gatherDir, "namespaces", namespace, subdir)
	for _, gvr := range gvrs {
		list, err := params.DynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			params.Logger.Debug("failed to list namespaced resource", "namespace", namespace, "resource", gvr.Resource, "error", err)
			continue
		}
		if len(list.Items) == 0 {
			continue
		}
		if err := os.MkdirAll(base, 0o755); err != nil {
			continue
		}
		m.writeUnstructuredList(list, filepath.Join(base, gvr.Resource+".json"))
	}
}

func (m *mustGather) collectRegionalCRs(ctx context.Context, params *ExecutionParams, gatherDir string) {
	m.collectNamespacedDynamicResources(ctx, params, "hyperfleet", "hyperfleet.io", []schema.GroupVersionResource{
		{Group: "hyperfleet.io", Version: "v1alpha1", Resource: "clusters"},
		{Group: "hyperfleet.io", Version: "v1alpha1", Resource: "nodepools"},
		{Group: "hyperfleet.io", Version: "v1alpha1", Resource: "placements"},
		{Group: "hyperfleet.io", Version: "v1alpha1", Resource: "managementclusters"},
	}, gatherDir)
}

func (m *mustGather) collectKarpenterCRs(ctx context.Context, params *ExecutionParams, gatherDir string) {
	m.collectNamespacedDynamicResources(ctx, params, "karpenter", "karpenter.sh", []schema.GroupVersionResource{
		{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"},
		{Group: "karpenter.sh", Version: "v1", Resource: "nodeclaims"},
	}, gatherDir)
	m.collectNamespacedDynamicResources(ctx, params, "karpenter", "karpenter.k8s.aws", []schema.GroupVersionResource{
		{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"},
	}, gatherDir)
}

// collectDirectDiagnostics gathers pod logs, events, and resource YAMLs via K8s API.
// Output structure matches oc adm must-gather / osdctl inner layout for omc/omg tools:
//
//	namespaces/<ns>/pods/<pod>/pod.yaml + <container>-current.log + <container>-previous.log
//	namespaces/<ns>/events/events.json
//	namespaces/<ns>/deployments/<name>.json
//	namespaces/<ns>/statefulsets/<name>.json
//	namespaces/<ns>/daemonsets/<name>.json
//	namespaces/<ns>/configmaps/<name>.json
//	namespaces/<ns>/services/<name>.json
//	namespaces/<ns>/jobs/<name>.json
//	namespaces/<ns>/cronjobs/<name>.json
//	namespaces/<ns>/persistentvolumeclaims/<name>.json
//	namespaces/<ns>/networkpolicies/<name>.json
//	namespaces/<ns>/secrets/<name>.json (metadata only — .data stripped)
//	cluster-scoped-resources/<group>/<resource>.json
func (m *mustGather) collectDirectDiagnostics(ctx context.Context, params *ExecutionParams, namespaces []string, gatherDir string) {
	for _, ns := range namespaces {
		nsDir := filepath.Join(gatherDir, "namespaces", ns)
		if err := os.MkdirAll(nsDir, 0o755); err != nil {
			params.Logger.Warn("failed to create namespace dir", "namespace", ns, "error", err)
			continue
		}

		m.collectPodLogs(ctx, params, ns, nsDir)
		m.collectEvents(ctx, params, ns, nsDir)
		m.collectAppsResources(ctx, params, ns, nsDir)
		m.collectConfigMaps(ctx, params, ns, nsDir)
		m.collectServices(ctx, params, ns, nsDir)
		m.collectBatchResources(ctx, params, ns, nsDir)
		m.collectPVCs(ctx, params, ns, nsDir)
		m.collectNetworkPolicies(ctx, params, ns, nsDir)
		m.collectSecretsMetadata(ctx, params, ns, nsDir)
	}
}

func (m *mustGather) collectPodLogs(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	podsDir := filepath.Join(nsDir, "pods")
	if err := os.MkdirAll(podsDir, 0o755); err != nil {
		params.Logger.Warn("failed to create pods dir", "namespace", namespace, "error", err)
		return
	}

	pods, err := params.KubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list pods for logs", "namespace", namespace, "error", err)
		return
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		podDir := filepath.Join(podsDir, pod.Name)
		if err := os.MkdirAll(podDir, 0o755); err != nil {
			continue
		}

		// Write pod.yaml alongside logs for omc compatibility
		podData, err := json.MarshalIndent(pod, "", "  ")
		if err == nil {
			_ = os.WriteFile(filepath.Join(podDir, "pod.yaml"), podData, 0o644)
		}

		for _, container := range pod.Spec.Containers {
			m.fetchContainerLogs(ctx, params, namespace, pod.Name, container.Name, podDir, false)
			m.fetchContainerLogs(ctx, params, namespace, pod.Name, container.Name, podDir, true)
		}
		for _, container := range pod.Spec.InitContainers {
			m.fetchContainerLogs(ctx, params, namespace, pod.Name, container.Name, podDir, false)
		}
	}
}

func (m *mustGather) fetchContainerLogs(ctx context.Context, params *ExecutionParams, namespace, podName, containerName, podDir string, previous bool) {
	opts := &corev1.PodLogOptions{
		Container: containerName,
		Previous:  previous,
	}

	req := params.KubeClient.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		if !errors.IsNotFound(err) && !strings.Contains(err.Error(), "previous terminated") {
			params.Logger.Debug("failed to get logs", "pod", podName, "container", containerName, "previous", previous, "error", err)
		}
		return
	}
	defer stream.Close()

	logData, err := io.ReadAll(stream)
	if err != nil || len(logData) == 0 {
		return
	}

	suffix := "current.log"
	if previous {
		suffix = "previous.log"
	}
	filename := filepath.Join(podDir, fmt.Sprintf("%s-%s", containerName, suffix))
	_ = os.WriteFile(filename, logData, 0o644)
}

func (m *mustGather) collectEvents(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	eventsDir := filepath.Join(nsDir, "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		params.Logger.Warn("failed to create events dir", "namespace", namespace, "error", err)
		return
	}

	events, err := params.KubeClient.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list events", "namespace", namespace, "error", err)
		return
	}

	if len(events.Items) == 0 {
		return
	}

	eventsData, err := json.MarshalIndent(events.Items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(eventsDir, "events.json"), eventsData, 0o644)
}

// namedResource is a name/object pair for dumpNamedResources.
type namedResource struct {
	Name   string
	Object interface{}
}

// dumpNamedResources serialises each item as <name>.json under nsDir/<subdir>.
// The directory is created lazily (only when items > 0) and MkdirAll errors
// are logged-and-skipped per resource type so one failure doesn't abort others.
func dumpNamedResources(logger *slog.Logger, nsDir, subdir string, items []namedResource) {
	if len(items) == 0 {
		return
	}
	dir := filepath.Join(nsDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("failed to create resource dir", "dir", dir, "error", err)
		return
	}
	for _, item := range items {
		data, err := json.MarshalIndent(item.Object, "", "  ")
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(dir, item.Name+".json"), data, 0o644)
	}
}

// dumpListItems converts a typed slice to namedResources and writes each as <name>.json.
// This generic helper eliminates the repetitive make→for→namedResource→dump boilerplate.
func dumpListItems[T any](logger *slog.Logger, nsDir, subdir string, items []T, nameOf func(*T) string) {
	named := make([]namedResource, len(items))
	for i := range items {
		named[i] = namedResource{Name: nameOf(&items[i]), Object: &items[i]}
	}
	dumpNamedResources(logger, nsDir, subdir, named)
}

func (m *mustGather) collectAppsResources(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	if list, err := params.KubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		params.Logger.Warn("failed to list deployments", "namespace", namespace, "error", err)
	} else {
		dumpListItems(params.Logger, nsDir, "deployments", list.Items, func(d *appsv1.Deployment) string { return d.Name })
	}

	if list, err := params.KubeClient.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		params.Logger.Warn("failed to list statefulsets", "namespace", namespace, "error", err)
	} else {
		dumpListItems(params.Logger, nsDir, "statefulsets", list.Items, func(s *appsv1.StatefulSet) string { return s.Name })
	}

	if list, err := params.KubeClient.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		params.Logger.Warn("failed to list daemonsets", "namespace", namespace, "error", err)
	} else {
		dumpListItems(params.Logger, nsDir, "daemonsets", list.Items, func(d *appsv1.DaemonSet) string { return d.Name })
	}
}

func (m *mustGather) collectConfigMaps(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	list, err := params.KubeClient.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list configmaps", "namespace", namespace, "error", err)
		return
	}
	dumpListItems(params.Logger, nsDir, "configmaps", list.Items, func(cm *corev1.ConfigMap) string { return cm.Name })
}

func (m *mustGather) collectServices(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	list, err := params.KubeClient.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list services", "namespace", namespace, "error", err)
		return
	}
	dumpListItems(params.Logger, nsDir, "services", list.Items, func(s *corev1.Service) string { return s.Name })
}

func (m *mustGather) collectBatchResources(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	if list, err := params.KubeClient.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		params.Logger.Warn("failed to list jobs", "namespace", namespace, "error", err)
	} else {
		dumpListItems(params.Logger, nsDir, "jobs", list.Items, func(j *batchv1.Job) string { return j.Name })
	}

	if list, err := params.KubeClient.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		params.Logger.Warn("failed to list cronjobs", "namespace", namespace, "error", err)
	} else {
		dumpListItems(params.Logger, nsDir, "cronjobs", list.Items, func(cj *batchv1.CronJob) string { return cj.Name })
	}
}

func (m *mustGather) collectPVCs(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	list, err := params.KubeClient.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list persistent volume claims", "namespace", namespace, "error", err)
		return
	}
	dumpListItems(params.Logger, nsDir, "persistentvolumeclaims", list.Items, func(p *corev1.PersistentVolumeClaim) string { return p.Name })
}

func (m *mustGather) collectNetworkPolicies(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	list, err := params.KubeClient.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list network policies", "namespace", namespace, "error", err)
		return
	}
	dumpListItems(params.Logger, nsDir, "networkpolicies", list.Items, func(p *networkingv1.NetworkPolicy) string { return p.Name })
}

func (m *mustGather) collectSecretsMetadata(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	secrets, err := params.KubeClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list secrets", "namespace", namespace, "error", err)
		return
	}
	items := make([]namedResource, len(secrets.Items))
	for i := range secrets.Items {
		items[i] = namedResource{Name: secrets.Items[i].Name, Object: redactSecretData(&secrets.Items[i])}
	}
	dumpNamedResources(params.Logger, nsDir, "secrets", items)
}

func redactSecretData(secret *corev1.Secret) *corev1.Secret {
	copy := secret.DeepCopy()
	copy.Data = nil
	copy.StringData = nil
	delete(copy.Annotations, corev1.LastAppliedConfigAnnotation)
	return copy
}

func (m *mustGather) writeUnstructuredList(list *unstructured.UnstructuredList, path string) {
	data, err := json.MarshalIndent(list.Items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// createTarball creates a gzipped tar archive from a source directory.
func createTarball(srcDir, dstPath string) (mustGatherArtifacts, error) {
	artifacts := mustGatherArtifacts{Tarball: filepath.Base(dstPath)}

	file, err := os.Create(dstPath)
	if err != nil {
		return artifacts, fmt.Errorf("creating tarball file: %w", err)
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// When dst lives under srcDir (e.g. output/output.tar.gz), skip self-inclusion.
		if path == dstPath {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tarWriter, f)
		return err
	})
	if err != nil {
		return artifacts, err
	}

	if err := tarWriter.Close(); err != nil {
		return artifacts, err
	}
	if err := gzWriter.Close(); err != nil {
		return artifacts, err
	}

	stat, err := os.Stat(dstPath)
	if err != nil {
		return artifacts, fmt.Errorf("stat tarball: %w", err)
	}
	artifacts.TarballBytes = stat.Size()
	return artifacts, nil
}

// extractTarGz extracts a gzipped tar archive from a reader into a destination directory.
func extractTarGz(r io.Reader, dstDir string) error {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		target := filepath.Join(dstDir, header.Name)
		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dstDir)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tarReader); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func parseNamespaceList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func truncateID(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func buildMustGatherSummary(id clusterIdentity, targets []string, skipImage bool, gatherImageErr error) string {
	label := strings.Join(targets, ",")
	if id.HCNamespace != "" && id.HostedClusterName != "" {
		summary := fmt.Sprintf("Must-gather [%s] collected for %s/%s", label, id.HCNamespace, id.HostedClusterName)
		switch {
		case skipImage || !gatherIncludes(targets, "hcp"):
			if !gatherIncludes(targets, "hcp") {
				summary += " (platform dump only)"
			} else {
				summary += " (gather image skipped, log supplement only)"
			}
		case gatherImageErr != nil:
			summary += " (gather image failed, log supplement collected)"
		}
		return summary
	}
	return fmt.Sprintf("Must-gather [%s] platform dump completed", label)
}

func logMustGatherReport(logger *slog.Logger, targets []string, skipImage bool, gatherImageErr error, namespaces []string, tar mustGatherArtifacts) {
	gatherImageStatus := "skipped"
	if gatherIncludes(targets, "hcp") {
		gatherImageStatus = "ok"
		if skipImage {
			gatherImageStatus = "skipped"
		} else if gatherImageErr != nil {
			gatherImageStatus = "failed"
		}
	}
	attrs := []any{
		"event", "must_gather_report",
		"gather", strings.Join(targets, ","),
		"gather_image", gatherImageStatus,
		"hcp_logs_dump", "ok",
		"namespaces", namespaces,
		"tarball", tar.Tarball,
		"tarball_size", formatHumanBytes(tar.TarballBytes),
	}
	if gatherImageErr != nil {
		attrs = append(attrs, "gather_image_error", gatherImageErr.Error())
	}
	logger.Info("must-gather collection report", attrs...)
}

func formatHumanBytes(n int64) string {
	switch {
	case n == 0:
		return "0B"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
}

func mustGatherPodName(executionID string) string {
	return fmt.Sprintf("must-gather-%s", truncateID(executionID, 51))
}

// hcNamespaceFromCPNamespace derives the HC namespace from the control-plane namespace
// when it follows the cluster-<uuid>-<name> convention (strips the -<name> suffix).
func hcNamespaceFromCPNamespace(cpNamespace, hostedClusterName string) string {
	suffix := "-" + hostedClusterName
	if strings.HasSuffix(cpNamespace, suffix) {
		return strings.TrimSuffix(cpNamespace, suffix)
	}
	return cpNamespace
}

func pollTimeoutFromContext(ctx context.Context, fallback time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return fallback
	}
	remaining := time.Until(deadline) - directDumpReserve
	if remaining < podWaitInterval {
		return podWaitInterval
	}
	return remaining
}
