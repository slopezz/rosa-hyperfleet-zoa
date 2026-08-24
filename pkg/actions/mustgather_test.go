package actions

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func mustGatherTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestMustGatherMetadata(t *testing.T) {
	action := &mustGather{}
	meta := action.Metadata()

	t.Run("When checking metadata it should have correct name and mode", func(t *testing.T) {
		if meta.Name != "must_gather" {
			t.Errorf("expected name 'must_gather', got %q", meta.Name)
		}
		if meta.ExecutionMode != "async" {
			t.Errorf("expected mode 'async', got %q", meta.ExecutionMode)
		}
		if meta.Scope != "kube-api" {
			t.Errorf("expected scope 'kube-api', got %q", meta.Scope)
		}
		if meta.Type != "read" {
			t.Errorf("expected type 'read', got %q", meta.Type)
		}
	})

	t.Run("When checking RBAC it should be cluster-scoped with secret read", func(t *testing.T) {
		if meta.RBAC == nil {
			t.Fatal("expected RBAC to be defined")
		}
		if !meta.RBAC.ClusterScoped {
			t.Error("expected cluster-scoped RBAC")
		}
		if !meta.RBAC.AllowSecretRead {
			t.Error("expected AllowSecretRead to be true")
		}
		if len(meta.RBAC.Rules) == 0 {
			t.Error("expected at least one RBAC rule")
		}
	})

	t.Run("When checking parameters it should require namespace and hosted_cluster_name", func(t *testing.T) {
		requiredParams := map[string]bool{"namespace": false, "hosted_cluster_name": false}
		for _, p := range meta.Parameters {
			if _, ok := requiredParams[p.Name]; ok {
				if !p.Required {
					t.Errorf("parameter %q should be required", p.Name)
				}
				requiredParams[p.Name] = true
			}
		}
		for name, found := range requiredParams {
			if !found {
				t.Errorf("required parameter %q not declared", name)
			}
		}
	})
}

func TestMustGatherValidate(t *testing.T) {
	action := &mustGather{}

	t.Run("When namespace is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"hosted_cluster_name": "test-hc"},
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing namespace")
		}
	})

	t.Run("When hosted_cluster_name is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "ocm-test-123"},
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing hosted_cluster_name")
		}
	})

	t.Run("When kube client is nil it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "ocm-test-123", "hosted_cluster_name": "my-cluster"},
			KubeClient: nil,
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for nil kube client")
		}
	})

	t.Run("When all required params are set it should pass validation", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "ocm-test-123", "hosted_cluster_name": "my-cluster"},
			KubeClient: fake.NewClientset(),
			RESTConfig: testRESTConfig,
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
}

func TestMustGatherResolveTargetNamespaces(t *testing.T) {
	action := &mustGather{}

	t.Run("When no extra namespaces it should return HCP, CP, and default namespaces", func(t *testing.T) {
		ns := action.resolveTargetNamespaces("clusters-abc123", "my-cluster", nil)
		if len(ns) != 4 {
			t.Fatalf("expected 4 namespaces (hcp + cp + hypershift + cert-manager), got %d: %v", len(ns), ns)
		}
		if ns[0] != "clusters-abc123" {
			t.Errorf("expected first ns to be HCP namespace, got %q", ns[0])
		}
		if ns[1] != "clusters-abc123-my-cluster" {
			t.Errorf("expected second ns to be CP namespace, got %q", ns[1])
		}
		if ns[2] != "hypershift" {
			t.Errorf("expected third ns to be 'hypershift', got %q", ns[2])
		}
		if ns[3] != "cert-manager" {
			t.Errorf("expected fourth ns to be 'cert-manager', got %q", ns[3])
		}
	})

	t.Run("When extra namespaces provided it should include them without duplicates", func(t *testing.T) {
		extra := []string{"monitoring", "clusters-abc123", "custom-ns"}
		ns := action.resolveTargetNamespaces("clusters-abc123", "my-cluster", extra)
		// 4 defaults + 2 new (monitoring, custom-ns) — clusters-abc123 is deduped
		if len(ns) != 6 {
			t.Fatalf("expected 6 namespaces (deduped), got %d: %v", len(ns), ns)
		}
	})
}

func TestMustGatherCollectDirectDiagnostics(t *testing.T) {
	action := &mustGather{}

	t.Run("When pods exist it should collect their logs and YAMLs in osdctl-compatible format", func(t *testing.T) {
		client := fake.NewClientset(
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod-1", Namespace: "hcp-ns"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-apiserver", Namespace: "hcp-ns"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "kas"}},
				},
			},
			&corev1.Event{
				ObjectMeta:     metav1.ObjectMeta{Name: "test-event", Namespace: "hcp-ns"},
				InvolvedObject: corev1.ObjectReference{Name: "test-pod-1", Namespace: "hcp-ns"},
				Reason:         "Started",
				Message:        "Started container",
			},
		)

		tmpDir := t.TempDir()
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "hcp-ns", "hosted_cluster_name": "cluster1"},
			KubeClient: client,
			Logger:     mustGatherTestLogger(),
		}

		action.collectDirectDiagnostics(context.Background(), params, []string{"hcp-ns"}, tmpDir)

		nsDir := filepath.Join(tmpDir, "namespaces", "hcp-ns")
		if _, err := os.Stat(nsDir); os.IsNotExist(err) {
			t.Fatal("expected namespace directory to be created")
		}

		eventsFile := filepath.Join(nsDir, "events", "events.json")
		if _, err := os.Stat(eventsFile); os.IsNotExist(err) {
			t.Error("expected events/events.json to be created")
		}

		deploymentsDir := filepath.Join(nsDir, "deployments")
		if _, err := os.Stat(deploymentsDir); os.IsNotExist(err) {
			t.Error("expected deployments directory to be created")
		}
		if _, err := os.Stat(filepath.Join(deploymentsDir, "kube-apiserver.json")); os.IsNotExist(err) {
			t.Error("expected kube-apiserver.json deployment to be collected")
		}

		podDir := filepath.Join(nsDir, "pods", "test-pod-1")
		if _, err := os.Stat(podDir); os.IsNotExist(err) {
			t.Error("expected pods/test-pod-1 directory to be created")
		}
		if _, err := os.Stat(filepath.Join(podDir, "pod.yaml")); os.IsNotExist(err) {
			t.Error("expected pod.yaml to be written alongside logs")
		}
	})
}

func TestMustGatherCreateTarball(t *testing.T) {
	t.Run("When creating tarball from directory it should produce valid archive", func(t *testing.T) {
		srcDir := t.TempDir()
		subDir := filepath.Join(srcDir, "subdir")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("world"), 0o644); err != nil {
			t.Fatal(err)
		}

		dstPath := filepath.Join(t.TempDir(), "output.tar.gz")
		if err := createTarball(srcDir, dstPath); err != nil {
			t.Fatalf("createTarball failed: %v", err)
		}

		// Verify tarball is valid and contains expected files
		f, err := os.Open(dstPath)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		defer gz.Close()

		tr := tar.NewReader(gz)
		files := make(map[string]bool)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			files[header.Name] = true
		}

		if !files["test.txt"] {
			t.Error("expected test.txt in tarball")
		}
		if !files["subdir/nested.txt"] {
			t.Error("expected subdir/nested.txt in tarball")
		}
	})
}

func TestMustGatherExtractTarGz(t *testing.T) {
	t.Run("When extracting tarball it should recreate directory structure", func(t *testing.T) {
		srcDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(srcDir, "a", "b"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "a", "b", "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}

		tarPath := filepath.Join(t.TempDir(), "test.tar.gz")
		if err := createTarball(srcDir, tarPath); err != nil {
			t.Fatal(err)
		}

		dstDir := t.TempDir()
		f, err := os.Open(tarPath)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		if err := extractTarGz(f, dstDir); err != nil {
			t.Fatalf("extractTarGz failed: %v", err)
		}

		extracted := filepath.Join(dstDir, "a", "b", "file.txt")
		data, err := os.ReadFile(extracted)
		if err != nil {
			t.Fatalf("expected extracted file at %s: %v", extracted, err)
		}
		if string(data) != "content" {
			t.Errorf("expected 'content', got %q", string(data))
		}
	})
}

func TestMustGatherParseNamespaceList(t *testing.T) {
	t.Run("When empty string it should return nil", func(t *testing.T) {
		result := parseNamespaceList("")
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("When comma-separated it should return trimmed list", func(t *testing.T) {
		result := parseNamespaceList("ns1, ns2 , ns3")
		if len(result) != 3 {
			t.Fatalf("expected 3, got %d", len(result))
		}
		if result[0] != "ns1" || result[1] != "ns2" || result[2] != "ns3" {
			t.Errorf("unexpected result: %v", result)
		}
	})
}

func TestMustGatherExecuteSkipImage(t *testing.T) {
	t.Run("When skip_must_gather_image is true it should only collect direct diagnostics", func(t *testing.T) {
		client := fake.NewClientset(
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "apiserver", Namespace: "clusters-test-abc"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "kube-apiserver", Image: "kas:latest"}},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
		)

		tmpDir := t.TempDir()
		t.Setenv("ZOA_OUTPUT_DIR", tmpDir)

		params := &ExecutionParams{
			Params: map[string]string{
				"namespace":              "clusters-test-abc",
				"hosted_cluster_name":    "my-hc",
				"skip_must_gather_image": "true",
			},
			ExecutionID: "test-exec-001",
			KubeClient:  client,
			RESTConfig:  testRESTConfig,
			Logger:      mustGatherTestLogger(),
		}

		action := &mustGather{}
		result, err := action.Execute(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatal("expected success")
		}
		if result.Summary == "" {
			t.Error("expected non-empty summary")
		}
		if result.Output != nil {
			t.Error("expected nil Output for tar.gz-producing action")
		}

		tarball := filepath.Join(tmpDir, "output.tar.gz")
		if _, err := os.Stat(tarball); os.IsNotExist(err) {
			t.Error("expected output.tar.gz to be created")
		}
	})
}

// testRESTConfig is a minimal rest.Config for validation tests.
var testRESTConfig = &rest.Config{Host: "https://localhost:6443"}
