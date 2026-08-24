package actions

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	mustGatherImage     = "quay.io/stolostron/must-gather:latest"
	mustGatherOutputDir = "/must-gather"
	mustGatherSentinel  = "/tmp/gather-done"
	podWaitTimeout      = 240 * time.Second
	podWaitInterval     = 5 * time.Second
	execCopyTimeout     = 120 * time.Second
)

func init() {
	Register(&mustGather{})
}

type mustGather struct{}

func (m *mustGather) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:          "must_gather",
		Scope:         "kube-api",
		Type:          "read",
		ExecutionMode: "async",
		Description: "Collect diagnostic must-gather bundle from a HostedCluster. " +
			"Runs the ACM must-gather image in a child pod, collects pod logs and events " +
			"from control plane namespaces, and packages everything into a tarball.",
		Authorization:  AuthorizationConfig{Approval: "none"},
		TimeoutSeconds: 295,
		Parameters: []ParameterDef{
			{Name: "namespace", Required: true, Description: "HCP namespace on the management cluster (e.g. clusters-<cluster-id>)"},
			{Name: "hosted_cluster_name", Required: true, Description: "Name of the HostedCluster resource"},
			{Name: "extra_namespaces", Description: "Comma-separated additional namespaces to collect logs/events from"},
			{Name: "skip_must_gather_image", Default: "false", Description: "Skip running the must-gather image (collect only pod logs and events)"},
		},
		RBAC: &RBACConfig{
			ClusterScoped:   true,
			AllowSecretRead: true,
			Rules: []RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "create", "delete"}},
				{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"get"}},
				{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
				{APIGroups: []string{""}, Resources: []string{"events"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{""}, Resources: []string{"configmaps", "services", "endpoints", "persistentvolumeclaims", "persistentvolumes"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{""}, Resources: []string{"serviceaccounts"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets", "replicasets"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"batch"}, Resources: []string{"jobs", "cronjobs"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"policy"}, Resources: []string{"poddisruptionbudgets"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"networking.k8s.io"}, Resources: []string{"networkpolicies"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"roles", "rolebindings", "clusterroles", "clusterrolebindings"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"storage.k8s.io"}, Resources: []string{"storageclasses", "volumeattachments"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificates", "issuers", "clusterissuers"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"karpenter.sh"}, Resources: []string{"nodepools", "nodeclaims"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"karpenter.k8s.aws"}, Resources: []string{"ec2nodeclasses"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"hypershift.openshift.io"}, Resources: []string{"hostedclusters", "hostedcontrolplanes", "nodepools"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"cluster.x-k8s.io"}, Resources: []string{"clusters", "machines", "machinedeployments", "machinesets"}, Verbs: []string{"get", "list"}},
				{APIGroups: []string{"hypershift.openshift.io"}, Resources: []string{"awsendpointservices"}, Verbs: []string{"get", "list"}},
			},
		},
	}
}

func (m *mustGather) Validate(_ context.Context, params *ExecutionParams) error {
	if err := ValidateRequiredParams(m.Metadata(), params.Params); err != nil {
		return err
	}

	if params.KubeClient == nil {
		return fmt.Errorf("kubernetes client is required")
	}
	if params.RESTConfig == nil {
		return fmt.Errorf("REST config is required for pod exec")
	}

	return nil
}

func (m *mustGather) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	hcpNamespace := params.Params["namespace"]
	hostedClusterName := params.Params["hosted_cluster_name"]
	skipImage := params.Params["skip_must_gather_image"] == "true"
	extraNamespaces := parseNamespaceList(params.Params["extra_namespaces"])

	params.Logger.Info("starting must-gather collection",
		"namespace", hcpNamespace,
		"hosted_cluster", hostedClusterName,
		"skip_image", skipImage,
	)

	outputDir := "/output"
	if envDir := os.Getenv("ZOA_OUTPUT_DIR"); envDir != "" {
		outputDir = envDir
	}
	gatherDir := filepath.Join(outputDir, "must-gather")
	if err := os.MkdirAll(gatherDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating gather directory: %w", err)
	}

	var phase1Err error
	if !skipImage {
		phase1Err = m.runMustGatherPod(ctx, params, hcpNamespace, hostedClusterName, gatherDir)
		if phase1Err != nil {
			params.Logger.Warn("must-gather image collection failed, continuing with direct collection", "error", phase1Err)
		}
	}

	// Phase 2: collect pod logs, events, and resource YAMLs directly via K8s API
	targetNamespaces := m.resolveTargetNamespaces(hcpNamespace, hostedClusterName, extraNamespaces)
	m.collectDirectDiagnostics(ctx, params, targetNamespaces, gatherDir)

	// Package into output.tar.gz — the canonical artifact name recognized by
	// the ZOA runner/executor for tar.gz detection via ArtifactSizes().
	tarballPath := filepath.Join(outputDir, "output.tar.gz")
	if err := createTarball(gatherDir, tarballPath); err != nil {
		return nil, fmt.Errorf("creating tarball: %w", err)
	}

	summary := fmt.Sprintf("Must-gather bundle collected for %s/%s", hcpNamespace, hostedClusterName)
	if phase1Err != nil {
		summary += " (must-gather image failed, direct collection only)"
	}

	// Return nil Output so the runner skips writing output.json, allowing
	// ArtifactSizes() to detect output.tar.gz as the primary artifact.
	return &ActionResult{
		Success: true,
		Output:  nil,
		Summary: summary,
	}, nil
}

// runMustGatherPod creates and manages the must-gather child pod.
func (m *mustGather) runMustGatherPod(ctx context.Context, params *ExecutionParams, hcpNamespace, hostedClusterName, gatherDir string) error {
	podName := fmt.Sprintf("must-gather-%s", truncateID(hcpNamespace, 40))
	namespace := os.Getenv("ZOA_JOBS_NAMESPACE")
	if namespace == "" {
		namespace = "zoa-jobs"
	}

	saName := fmt.Sprintf("zoa-exec-%s", params.ExecutionID)
	params.Logger.Info("creating must-gather pod", "pod", podName, "namespace", namespace, "service_account", saName)

	gatherCmd := fmt.Sprintf(
		"/usr/bin/gather_hcp --hcp-namespace=%s --hosted-cluster-name=%s 2>&1 || true; touch %s; sleep 600",
		hcpNamespace, hostedClusterName, mustGatherSentinel,
	)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "zoa",
				"zoa.openshift.io/component":   "must-gather",
				"zoa.openshift.io/execution":   truncateID(params.ExecutionID, 63),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: saName,
			Containers: []corev1.Container{
				{
					Name:    "must-gather",
					Image:   mustGatherImage,
					Command: []string{"/bin/sh", "-c", gatherCmd},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "gather-output", MountPath: mustGatherOutputDir},
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

	// Clean up any pre-existing pod
	_ = params.KubeClient.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})

	_, err := params.KubeClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating must-gather pod: %w", err)
	}

	defer func() {
		params.Logger.Info("cleaning up must-gather pod")
		_ = params.KubeClient.CoreV1().Pods(namespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
	}()

	// Wait for gather to complete (sentinel file)
	if err := m.waitForGatherCompletion(ctx, params, namespace, podName); err != nil {
		return fmt.Errorf("waiting for gather completion: %w", err)
	}

	// Copy output from the pod
	if err := m.copyFromPod(ctx, params, namespace, podName, mustGatherOutputDir, gatherDir); err != nil {
		return fmt.Errorf("copying must-gather output: %w", err)
	}

	return nil
}

func (m *mustGather) waitForGatherCompletion(ctx context.Context, params *ExecutionParams, namespace, podName string) error {
	params.Logger.Info("waiting for must-gather to complete")

	return wait.PollUntilContextTimeout(ctx, podWaitInterval, podWaitTimeout, true, func(ctx context.Context) (bool, error) {
		pod, err := params.KubeClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("getting pod status: %w", err)
		}

		switch pod.Status.Phase {
		case corev1.PodFailed:
			return false, fmt.Errorf("must-gather pod failed")
		case corev1.PodSucceeded:
			return true, nil
		case corev1.PodRunning:
			// Check sentinel file
			stdout, _, execErr := m.execInPod(ctx, params, namespace, podName, "must-gather",
				[]string{"test", "-f", mustGatherSentinel})
			if execErr == nil && stdout != "" || execErr == nil {
				// test -f returns 0 if file exists
				_, _, checkErr := m.execInPod(ctx, params, namespace, podName, "must-gather",
					[]string{"cat", mustGatherSentinel})
				if checkErr == nil {
					return true, nil
				}
			}
			return false, nil
		default:
			return false, nil
		}
	})
}

// copyFromPod copies files from a pod container to local filesystem using tar exec.
func (m *mustGather) copyFromPod(ctx context.Context, params *ExecutionParams, namespace, podName, srcPath, dstPath string) error {
	params.Logger.Info("copying data from must-gather pod", "src", srcPath, "dst", dstPath)

	tarCmd := []string{"tar", "czf", "-", "-C", srcPath, "."}
	stdout, _, err := m.execInPodRaw(ctx, params, namespace, podName, "must-gather", tarCmd)
	if err != nil {
		return fmt.Errorf("exec tar in pod: %w", err)
	}

	return extractTarGz(bytes.NewReader(stdout), dstPath)
}

// execInPod runs a command in a pod container and returns stdout/stderr.
func (m *mustGather) execInPod(ctx context.Context, params *ExecutionParams, namespace, podName, container string, command []string) (string, string, error) {
	stdout, stderr, err := m.execInPodRaw(ctx, params, namespace, podName, container, command)
	return string(stdout), string(stderr), err
}

func (m *mustGather) execInPodRaw(ctx context.Context, params *ExecutionParams, namespace, podName, container string, command []string) ([]byte, []byte, error) {
	req := params.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(params.RESTConfig, "POST", req.URL())
	if err != nil {
		return nil, nil, fmt.Errorf("creating SPDY executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	execCtx, cancel := context.WithTimeout(ctx, execCopyTimeout)
	defer cancel()

	err = exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("stream exec: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}

// resolveTargetNamespaces determines which namespaces to collect diagnostics from.
// Default set matches osdctl hcp must-gather V1 targets: HCP ns, hypershift operator,
// and cert-manager (critical for TLS health of control plane components).
func (m *mustGather) resolveTargetNamespaces(hcpNamespace, hostedClusterName string, extra []string) []string {
	namespaces := []string{hcpNamespace}

	// Add the control plane namespace (convention: <hcp-ns>-<cluster-name>)
	cpNamespace := fmt.Sprintf("%s-%s", hcpNamespace, hostedClusterName)
	namespaces = append(namespaces, cpNamespace)

	defaults := []string{"hypershift", "cert-manager"}
	namespaces = append(namespaces, defaults...)

	seen := make(map[string]bool, len(namespaces))
	for _, ns := range namespaces {
		seen[ns] = true
	}
	for _, ns := range extra {
		ns = strings.TrimSpace(ns)
		if ns != "" && !seen[ns] {
			namespaces = append(namespaces, ns)
			seen[ns] = true
		}
	}

	return namespaces
}

// collectDirectDiagnostics gathers pod logs, events, and resource YAMLs via K8s API.
// Output structure mirrors osdctl hcp must-gather for compatibility with omc/omg tools:
//
//	namespaces/<ns>/pods/<pod>/pod.yaml + <container>-current.log + <container>-previous.log
//	namespaces/<ns>/events/events.json
//	namespaces/<ns>/deployments/<name>.json
//	namespaces/<ns>/statefulsets/<name>.json
//	cluster-scoped-resources/hypershift.openshift.io/...
func (m *mustGather) collectDirectDiagnostics(ctx context.Context, params *ExecutionParams, namespaces []string, gatherDir string) {
	for _, ns := range namespaces {
		nsDir := filepath.Join(gatherDir, "namespaces", ns)
		if err := os.MkdirAll(nsDir, 0o755); err != nil {
			params.Logger.Warn("failed to create namespace dir", "namespace", ns, "error", err)
			continue
		}

		m.collectPodLogs(ctx, params, ns, nsDir)
		m.collectEvents(ctx, params, ns, nsDir)
		m.collectDeployments(ctx, params, ns, nsDir)
		m.collectPodYAMLs(ctx, params, ns, nsDir)
	}

	// Collect cluster-scoped HyperShift resources
	m.collectHyperShiftResources(ctx, params, gatherDir)
}

func (m *mustGather) collectPodLogs(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	podsDir := filepath.Join(nsDir, "pods")
	if err := os.MkdirAll(podsDir, 0o755); err != nil {
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

func (m *mustGather) collectDeployments(ctx context.Context, params *ExecutionParams, namespace, nsDir string) {
	deploymentsDir := filepath.Join(nsDir, "deployments")
	if err := os.MkdirAll(deploymentsDir, 0o755); err != nil {
		return
	}

	deployments, err := params.KubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		params.Logger.Warn("failed to list deployments", "namespace", namespace, "error", err)
		return
	}

	for i := range deployments.Items {
		dep := &deployments.Items[i]
		data, err := json.MarshalIndent(dep, "", "  ")
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(deploymentsDir, dep.Name+".json"), data, 0o644)
	}

	// Also collect StatefulSets
	stsDir := filepath.Join(nsDir, "statefulsets")
	if err := os.MkdirAll(stsDir, 0o755); err != nil {
		return
	}

	statefulsets, err := params.KubeClient.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for i := range statefulsets.Items {
		sts := &statefulsets.Items[i]
		data, err := json.MarshalIndent(sts, "", "  ")
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(stsDir, sts.Name+".json"), data, 0o644)
	}
}

func (m *mustGather) collectPodYAMLs(_ context.Context, _ *ExecutionParams, _, _ string) {
	// Pod YAMLs are now written inline by collectPodLogs (pods/<name>/pod.yaml).
	// This method is retained as a no-op for interface compatibility during tests.
}

func (m *mustGather) collectHyperShiftResources(ctx context.Context, params *ExecutionParams, gatherDir string) {
	if params.DynamicClient == nil {
		return
	}

	hsDir := filepath.Join(gatherDir, "cluster-scoped-resources", "hypershift.openshift.io")
	if err := os.MkdirAll(hsDir, 0o755); err != nil {
		return
	}

	resources := []struct {
		gvr  schema.GroupVersionResource
		name string
	}{
		{schema.GroupVersionResource{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedclusters"}, "hostedclusters"},
		{schema.GroupVersionResource{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedcontrolplanes"}, "hostedcontrolplanes"},
		{schema.GroupVersionResource{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "nodepools"}, "nodepools"},
	}

	for _, r := range resources {
		list, err := params.DynamicClient.Resource(r.gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			params.Logger.Debug("failed to list HyperShift resource", "resource", r.name, "error", err)
			continue
		}
		m.writeUnstructuredList(list, filepath.Join(hsDir, r.name+".json"))
	}
}

func (m *mustGather) writeUnstructuredList(list *unstructured.UnstructuredList, path string) {
	data, err := json.MarshalIndent(list.Items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// createTarball creates a gzipped tar archive from a source directory.
func createTarball(srcDir, dstPath string) error {
	file, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating tarball file: %w", err)
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

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
