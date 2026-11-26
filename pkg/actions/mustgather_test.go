package actions

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func mustGatherHostedClusterClient(name, namespace string) *dynamicfake.FakeDynamicClient {
	hc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "hypershift.openshift.io/v1beta1",
			"kind":       "HostedCluster",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedclusters"}:      "HostedClusterList",
		{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedcontrolplanes"}: "HostedControlPlaneList",
		{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "nodepools"}:           "NodePoolList",
		{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machines"}:                   "MachineList",
		{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machinedeployments"}:         "MachineDeploymentList",
		{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machinesets"}:                "MachineSetList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, hc)
}

func mustGatherTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestRedactSecretData_ItShouldStripDataFields(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: "cert-manager",
			Annotations: map[string]string{
				corev1.LastAppliedConfigAnnotation: `{"data":{"token":"c2VjcmV0LXZhbHVl"}}`,
				"other-annotation":                 "keep-me",
			},
		},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"token": []byte("secret-value")},
		StringData: map[string]string{"other": "also-secret"},
	}

	redacted := redactSecretData(secret)
	if len(redacted.Data) != 0 {
		t.Fatal("expected data to be stripped")
	}
	if len(redacted.StringData) != 0 {
		t.Fatal("expected stringData to be stripped")
	}
	if _, ok := redacted.Annotations[corev1.LastAppliedConfigAnnotation]; ok {
		t.Fatal("expected last-applied-configuration annotation to be stripped")
	}
	if redacted.Annotations["other-annotation"] != "keep-me" {
		t.Fatal("expected non-sensitive annotations to be preserved")
	}
	if redacted.Name != "pull-secret" {
		t.Fatalf("expected metadata preserved, got name %q", redacted.Name)
	}
	if len(secret.Data) != 1 {
		t.Fatal("redactSecretData should not mutate the original secret")
	}
	if _, ok := secret.Annotations[corev1.LastAppliedConfigAnnotation]; !ok {
		t.Fatal("redactSecretData should not mutate the original secret annotations")
	}
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
		if !meta.DisallowExecutionModeOverride {
			t.Error("expected DisallowExecutionModeOverride to be true")
		}
		if meta.TimeoutSeconds != mustGatherTimeout {
			t.Errorf("expected timeout %d, got %d", mustGatherTimeout, meta.TimeoutSeconds)
		}
		if meta.Scope != ScopeKube {
			t.Errorf("expected scope 'kube', got %q", meta.Scope)
		}
		if meta.Type != "read" {
			t.Errorf("expected type 'read', got %q", meta.Type)
		}
	})

	t.Run("When checking RBAC it should use diagnostic read plus gather verbs", func(t *testing.T) {
		if meta.RBAC == nil {
			t.Fatal("expected RBAC to be defined")
		}
		if !meta.RBAC.ClusterScoped {
			t.Error("expected cluster-scoped RBAC")
		}
		if !meta.RBAC.AllowSecretRead {
			t.Error("expected AllowSecretRead to be true")
		}

		hasDiagnosticRead := false
		hasPortForward := false
		hasPodLifecycle := false
		for _, rule := range meta.RBAC.Rules {
			if len(rule.APIGroups) == 1 && rule.APIGroups[0] == "*" &&
				len(rule.Resources) == 1 && rule.Resources[0] == "*" &&
				slices.Equal(rule.Verbs, []string{"get", "list"}) {
				hasDiagnosticRead = true
			}
			if len(rule.Resources) == 1 && rule.Resources[0] == "pods/portforward" &&
				slices.Equal(rule.Verbs, []string{"create"}) {
				hasPortForward = true
			}
			if len(rule.Resources) == 1 && rule.Resources[0] == "pods" &&
				slices.Equal(rule.Verbs, []string{"create", "delete"}) {
				hasPodLifecycle = true
			}
		}
		if !hasDiagnosticRead {
			t.Error("expected cluster-wide get/list rule for diagnostic read")
		}
		if !hasPortForward {
			t.Error("expected pods/portforward create for guest cluster dump")
		}
		if !hasPodLifecycle {
			t.Error("expected pods create/delete for must-gather pod lifecycle")
		}
	})

	t.Run("When checking parameters it should declare gather as required", func(t *testing.T) {
		wantRequired := map[string]bool{"gather": false}
		wantOptional := map[string]bool{"cluster_id": false, "extra_namespaces": false, "skip_must_gather_image": false}
		for _, p := range meta.Parameters {
			if _, ok := wantRequired[p.Name]; ok {
				wantRequired[p.Name] = true
				if !p.Required {
					t.Errorf("parameter %q should be required", p.Name)
				}
				if p.Default != "" {
					t.Errorf("parameter %q should not have a default", p.Name)
				}
			}
			if _, ok := wantOptional[p.Name]; ok {
				wantOptional[p.Name] = true
				if p.Required {
					t.Errorf("parameter %q should be optional", p.Name)
				}
			}
		}
		for name, found := range wantRequired {
			if !found {
				t.Errorf("expected parameter %q to be declared", name)
			}
		}
		for name, found := range wantOptional {
			if !found {
				t.Errorf("expected parameter %q to be declared", name)
			}
		}
		if meta.Parameters[0].Name != "gather" {
			t.Errorf("expected gather to be first parameter, got %q", meta.Parameters[0].Name)
		}
	})

	t.Run("When checking deployment targets it should register on rc and mc", func(t *testing.T) {
		want := []string{TargetTypeRC, TargetTypeMC}
		if len(meta.TargetTypes) != len(want) {
			t.Fatalf("expected %v deployment targets, got %v", want, meta.TargetTypes)
		}
		for _, target := range want {
			if !slices.Contains(meta.TargetTypes, target) {
				t.Errorf("expected deployment target %q in %v", target, meta.TargetTypes)
			}
		}
	})
}

func TestMustGatherValidate(t *testing.T) {
	action := &mustGather{}

	t.Run("When gather includes hcp without cluster_id it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"gather": "hcp"},
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing cluster_id")
		}
	})

	t.Run("When kube client is nil it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{
				"gather":                 "hcp",
				"cluster_id":             "test-123",
				"skip_must_gather_image": "true",
			},
			KubeClient:    fake.NewClientset(),
			DynamicClient: mustGatherHostedClusterClient("my-cluster", "cluster-test-123"),
			Logger:        mustGatherTestLogger(),
		}
		params.KubeClient = nil
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for nil kube client")
		}
	})

	t.Run("When cluster_id resolves a sole HostedCluster it should pass validation", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{
				"gather":                 "hcp",
				"cluster_id":             "test-123",
				"skip_must_gather_image": "true",
			},
			KubeClient:    fake.NewClientset(),
			DynamicClient: mustGatherHostedClusterClient("my-cluster", "cluster-test-123"),
			Logger:        mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
}

func TestMustGatherResolveCollectionNamespaces_WhenHCP_ItShouldReturnHCPAndPlatform(t *testing.T) {
	action := &mustGather{}
	id := clusterIdentity{
		HCNamespace:       "cluster-abc123",
		HostedClusterName: "my-cluster",
		CPNamespace:       "cluster-abc123-my-cluster",
	}

	t.Run("When no extra namespaces it should return HCP, CP, and default namespaces", func(t *testing.T) {
		ns := action.resolveCollectionNamespaces([]string{"hcp"}, id, nil)
		if len(ns) != 5 {
			t.Fatalf("expected 5 namespaces (hcp + cp + hypershift + cert-manager + kube-applier), got %d: %v", len(ns), ns)
		}
		if ns[0] != "cluster-abc123" {
			t.Errorf("expected first ns to be HCP namespace, got %q", ns[0])
		}
		if ns[1] != "cluster-abc123-my-cluster" {
			t.Errorf("expected second ns to be CP namespace, got %q", ns[1])
		}
		if ns[2] != "hypershift" {
			t.Errorf("expected third ns to be 'hypershift', got %q", ns[2])
		}
		if ns[3] != "cert-manager" {
			t.Errorf("expected fourth ns to be 'cert-manager', got %q", ns[3])
		}
		if ns[4] != "kube-applier" {
			t.Errorf("expected fifth ns to be 'kube-applier', got %q", ns[4])
		}
	})

	t.Run("When extra namespaces provided it should include them without duplicates", func(t *testing.T) {
		extra := []string{"monitoring", "cluster-abc123", "custom-ns"}
		ns := action.resolveCollectionNamespaces([]string{"hcp"}, id, extra)
		// 5 defaults + 2 new (monitoring, custom-ns) — cluster-abc123 is deduped
		if len(ns) != 7 {
			t.Fatalf("expected 7 namespaces (deduped), got %d: %v", len(ns), ns)
		}
	})
}

func TestMustGatherCollectDirectDiagnostics(t *testing.T) {
	action := &mustGather{}

	t.Run("When pods exist it should collect their logs and YAMLs in must-gather layout", func(t *testing.T) {
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
			&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "node-exporter", Namespace: "hcp-ns"},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "node-exporter"}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "operator-config", Namespace: "hcp-ns"},
				Data:       map[string]string{"logLevel": "debug"},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "pull-secret", Namespace: "hcp-ns"},
				Data:       map[string][]byte{"token": []byte("super-secret")},
			},
			&corev1.Event{
				ObjectMeta:     metav1.ObjectMeta{Name: "test-event", Namespace: "hcp-ns"},
				InvolvedObject: corev1.ObjectReference{Name: "test-pod-1", Namespace: "hcp-ns"},
				Reason:         "Started",
				Message:        "Started container",
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-apiserver", Namespace: "hcp-ns"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "kas"},
					Ports:    []corev1.ServicePort{{Port: 443}},
				},
			},
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "backup-job", Namespace: "hcp-ns"},
			},
			&batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{Name: "nightly-sync", Namespace: "hcp-ns"},
				Spec: batchv1.CronJobSpec{
					Schedule: "0 0 * * *",
					JobTemplate: batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyOnFailure,
									Containers:    []corev1.Container{{Name: "sync", Image: "sync:latest"}},
								},
							},
						},
					},
				},
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "etcd-data", Namespace: "hcp-ns"},
			},
			&networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-apiserver", Namespace: "hcp-ns"},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "kas"}},
				},
			},
		)

		tmpDir := t.TempDir()
		params := &ExecutionParams{
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

		daemonsetsDir := filepath.Join(nsDir, "daemonsets")
		if _, err := os.Stat(filepath.Join(daemonsetsDir, "node-exporter.json")); os.IsNotExist(err) {
			t.Error("expected node-exporter.json daemonset to be collected")
		}

		configmapsDir := filepath.Join(nsDir, "configmaps")
		if _, err := os.Stat(filepath.Join(configmapsDir, "operator-config.json")); os.IsNotExist(err) {
			t.Error("expected operator-config.json configmap to be collected")
		}

		secretFile := filepath.Join(nsDir, "secrets", "pull-secret.json")
		secretData, err := os.ReadFile(secretFile)
		if err != nil {
			t.Fatalf("expected pull-secret.json to be collected: %v", err)
		}
		var secret corev1.Secret
		if err := json.Unmarshal(secretData, &secret); err != nil {
			t.Fatalf("failed to parse secret json: %v", err)
		}
		if len(secret.Data) != 0 {
			t.Error("expected secret data to be redacted")
		}
		if secret.Name != "pull-secret" {
			t.Errorf("expected secret metadata name pull-secret, got %q", secret.Name)
		}

		podDir := filepath.Join(nsDir, "pods", "test-pod-1")
		if _, err := os.Stat(podDir); os.IsNotExist(err) {
			t.Error("expected pods/test-pod-1 directory to be created")
		}
		if _, err := os.Stat(filepath.Join(podDir, "pod.yaml")); os.IsNotExist(err) {
			t.Error("expected pod.yaml to be written alongside logs")
		}

		for _, check := range []struct {
			dir  string
			file string
		}{
			{"services", "kube-apiserver.json"},
			{"jobs", "backup-job.json"},
			{"cronjobs", "nightly-sync.json"},
			{"persistentvolumeclaims", "etcd-data.json"},
			{"networkpolicies", "allow-apiserver.json"},
		} {
			path := filepath.Join(nsDir, check.dir, check.file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("expected %s to be collected", path)
			}
		}
	})
}

func TestMustGatherCollectClusterStorage(t *testing.T) {
	t.Run("When collecting cluster storage it should write storage classes and persistent volumes", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			&storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "gp3"},
				Provisioner: "ebs.csi.aws.com",
			},
			&corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "pv-etcd-0"},
				Spec: corev1.PersistentVolumeSpec{
					Capacity: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		)

		tmpDir := t.TempDir()
		action := &mustGather{}
		action.collectClusterStorage(context.Background(), &ExecutionParams{
			KubeClient: client,
			Logger:     mustGatherTestLogger(),
		}, tmpDir)

		scFile := filepath.Join(tmpDir, "cluster-scoped-resources", "storage.k8s.io", "storageclasses.json")
		if _, err := os.Stat(scFile); os.IsNotExist(err) {
			t.Fatal("expected storageclasses.json to be created")
		}
		pvFile := filepath.Join(tmpDir, "cluster-scoped-resources", "core", "persistentvolumes.json")
		if _, err := os.Stat(pvFile); os.IsNotExist(err) {
			t.Fatal("expected persistentvolumes.json to be created")
		}
	})
}

func TestMustGatherCreateTarball(t *testing.T) {
	t.Run("When creating tarball from directory it should produce valid archive", func(t *testing.T) {
		root := t.TempDir()
		srcDir := filepath.Join(root, "hcp", "controlplane-demo")
		subDir := filepath.Join(srcDir, "namespaces", "kube-system")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "test.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}

		dstPath := filepath.Join(t.TempDir(), "output.tar.gz")
		info, err := createTarball(root, dstPath)
		if err != nil {
			t.Fatalf("createTarball failed: %v", err)
		}
		if info.Tarball != "output.tar.gz" {
			t.Errorf("expected tarball name output.tar.gz, got %q", info.Tarball)
		}

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

		if !files["hcp/controlplane-demo/namespaces/kube-system/test.txt"] {
			t.Errorf("expected osdctl-style path in tarball, got keys: %v", files)
		}
	})

	t.Run("When destination tarball is inside source dir it should not include itself", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		dstPath := filepath.Join(root, "output.tar.gz")
		if _, err := createTarball(root, dstPath); err != nil {
			t.Fatalf("createTarball failed: %v", err)
		}

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
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if header.Name == "output.tar.gz" {
				t.Fatal("tarball should not contain itself")
			}
		}
	})
}

func TestMustGatherExtractTarGz(t *testing.T) {
	t.Run("When extracting tarball it should recreate directory structure", func(t *testing.T) {
		root := t.TempDir()
		srcDir := filepath.Join(root, "mc", "namespaces", "default")
		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}

		tarPath := filepath.Join(t.TempDir(), "test.tar.gz")
		if _, err := createTarball(root, tarPath); err != nil {
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

		extracted := filepath.Join(dstDir, "mc", "namespaces", "default", "file.txt")
		data, err := os.ReadFile(extracted)
		if err != nil {
			t.Fatalf("expected extracted file at %s: %v", extracted, err)
		}
		if string(data) != "content" {
			t.Errorf("expected 'content', got %q", string(data))
		}
	})
}

func TestMustGatherHypershiftDumpScript(t *testing.T) {
	t.Run("When building dump script it should run hypershift dump with archive-dump false", func(t *testing.T) {
		script := mustGatherHypershiftDumpScript("cluster-abc123", "sergio")
		for _, want := range []string{
			`export BASE_COLLECTION_PATH="/must-gather"`,
			". /usr/bin/gather_utils",
			"parse_args hosted-cluster-namespace=cluster-abc123 hosted-cluster-name=sergio",
			"ensure_hypershift_cli",
			"--archive-dump=false",
			"--dump-guest-cluster=fail-on-error",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("expected script to contain %q, got:\n%s", want, script)
			}
		}
		if strings.Contains(script, "/usr/bin/gather ") {
			t.Errorf("script should not invoke /usr/bin/gather: %s", script)
		}
		if strings.Contains(script, "dump_hostedcluster") {
			t.Errorf("script should not use dump_hostedcluster (creates redundant inner tar): %s", script)
		}
	})
}

func TestValidateHCPDumpOutput(t *testing.T) {
	t.Run("When hostedcluster dir exists it should pass", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "hostedcluster-sergio"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := validateHCPDumpOutput(dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("When hypershift-dump marker exists it should pass", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "hypershift-dump.tar.gz"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := validateHCPDumpOutput(dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("When directory is empty it should fail", func(t *testing.T) {
		dir := t.TempDir()
		if err := validateHCPDumpOutput(dir); err == nil {
			t.Fatal("expected error for empty dump output")
		}
	})
}

func TestMustGatherResolveHostedClusterNamespace(t *testing.T) {
	t.Run("When namespace includes hosted cluster suffix it should trim for gather script", func(t *testing.T) {
		ns := hcNamespaceFromCPNamespace("cluster-abc123-my-cluster", "my-cluster")
		if ns != "cluster-abc123" {
			t.Errorf("expected trimmed namespace, got %q", ns)
		}
	})

	t.Run("When namespace has no suffix it should return unchanged", func(t *testing.T) {
		ns := hcNamespaceFromCPNamespace("cluster-abc123", "my-cluster")
		if ns != "cluster-abc123" {
			t.Errorf("expected unchanged namespace, got %q", ns)
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
	t.Run("When skip_must_gather_image is true it should collect hcp log supplement only", func(t *testing.T) {
		client := fake.NewClientset(
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "apiserver", Namespace: "cluster-test-abc-my-hc"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "kube-apiserver", Image: "kas:latest"}},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-apiserver", Namespace: "cluster-test-abc-my-hc"},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "kas"}},
				},
			},
			&corev1.Event{
				ObjectMeta:     metav1.ObjectMeta{Name: "dep-event", Namespace: "cluster-test-abc-my-hc"},
				InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "kube-apiserver", Namespace: "cluster-test-abc-my-hc"},
				Reason:         "ScalingReplicaSet",
				Type:           corev1.EventTypeNormal,
				Message:        "Scaled up",
			},
		)

		tmpDir := t.TempDir()
		t.Setenv("ZOA_OUTPUT_DIR", tmpDir)

		params := &ExecutionParams{
			Params: map[string]string{
				"gather":                 "hcp",
				"cluster_id":             "test-abc",
				"skip_must_gather_image": "true",
			},
			ExecutionID:   "test-exec-001",
			KubeClient:    client,
			DynamicClient: mustGatherHostedClusterClient("my-hc", "cluster-test-abc"),
			RESTConfig:    testRESTConfig,
			Logger:        mustGatherTestLogger(),
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

		// When image is skipped, collectDirectDiagnostics should collect CP namespace
		// under hcp/namespaces/<cpns>/ (richer than the old hcp-logs-dump path).
		podYAML := filepath.Join(tmpDir, "hcp", "namespaces", "cluster-test-abc-my-hc", "pods", "apiserver", "pod.yaml")
		if _, err := os.Stat(podYAML); os.IsNotExist(err) {
			t.Errorf("expected pod.yaml at %s", podYAML)
		}
		// Supplement platform namespaces should also be present
		for _, ns := range []string{"hypershift", "cert-manager", "kube-applier"} {
			nsDir := filepath.Join(tmpDir, "hcp", "namespaces", ns)
			if _, err := os.Stat(nsDir); os.IsNotExist(err) {
				t.Errorf("expected supplement namespace dir %s", nsDir)
			}
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "hcp", "controlplane-my-hc")); !os.IsNotExist(err) {
			t.Error("expected no controlplane direct dump when gather image skipped")
		}
	})
}

func TestMustGatherExecutionSuccess(t *testing.T) {
	t.Run("When gather image failed for hcp it should mark execution unsuccessful", func(t *testing.T) {
		if mustGatherExecutionSuccess([]string{"hcp"}, false, fmt.Errorf("copy failed")) {
			t.Fatal("expected failure when gather image required but failed")
		}
	})

	t.Run("When gather image skipped it should succeed with direct dump only", func(t *testing.T) {
		if !mustGatherExecutionSuccess([]string{"hcp"}, true, nil) {
			t.Fatal("expected success when skip image")
		}
	})

	t.Run("When gather image error but skip requested it should succeed", func(t *testing.T) {
		if !mustGatherExecutionSuccess([]string{"hcp"}, true, fmt.Errorf("ignored")) {
			t.Fatal("expected success when skip image even with spurious error")
		}
	})

	t.Run("When mc-only gather it should succeed regardless of gather image error", func(t *testing.T) {
		if !mustGatherExecutionSuccess([]string{"mc"}, false, fmt.Errorf("n/a")) {
			t.Fatal("expected success for mc-only")
		}
	})

	t.Run("When no gather image error it should succeed", func(t *testing.T) {
		if !mustGatherExecutionSuccess([]string{"hcp"}, false, nil) {
			t.Fatal("expected success")
		}
	})
}

func TestMustGatherPodName(t *testing.T) {
	t.Run("When execution IDs differ it should produce unique pod names", func(t *testing.T) {
		a := mustGatherPodName("exec-aaa-111")
		b := mustGatherPodName("exec-bbb-222")
		if a == b {
			t.Errorf("expected unique pod names, both got %q", a)
		}
		if !strings.HasPrefix(a, "must-gather-") {
			t.Errorf("expected must-gather- prefix, got %q", a)
		}
	})
}

func TestMustGatherPodFailureReason(t *testing.T) {
	t.Run("When init container exited non-zero it should include exit code and reason", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				InitContainerStatuses: []corev1.ContainerStatus{
					{
						Name: mustGatherInitContainer,
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1,
								Reason:   "Error",
								Message:  "gather script failed",
							},
						},
					},
				},
			},
		}
		reason := mustGatherPodFailureReason(pod)
		if !strings.Contains(reason, "exit 1") || !strings.Contains(reason, "gather script failed") {
			t.Errorf("unexpected reason: %q", reason)
		}
	})

	t.Run("When container exited non-zero it should include exit code and reason", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "must-gather",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1,
								Reason:   "Error",
								Message:  "gather script failed",
							},
						},
					},
				},
			},
		}
		reason := mustGatherPodFailureReason(pod)
		if !strings.Contains(reason, "exit 1") || !strings.Contains(reason, "gather script failed") {
			t.Errorf("unexpected reason: %q", reason)
		}
	})

	t.Run("When no container status it should return generic failure", func(t *testing.T) {
		reason := mustGatherPodFailureReason(&corev1.Pod{})
		if reason != "must-gather pod failed" {
			t.Errorf("unexpected reason: %q", reason)
		}
	})
}

func TestMustGatherInitContainerStatus(t *testing.T) {
	t.Run("When init container succeeded and hold is running it should be ready to copy", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				InitContainerStatuses: []corev1.ContainerStatus{
					{
						Name: mustGatherInitContainer,
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
						},
					},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: mustGatherHoldContainer,
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		}
		if !mustGatherInitContainerSucceeded(pod) {
			t.Fatal("expected init container success")
		}
		if err := mustGatherInitContainerFailure(pod); err != nil {
			t.Fatalf("unexpected init failure: %v", err)
		}
		if !containerRunning(pod, mustGatherHoldContainer) {
			t.Fatal("expected hold container running")
		}
	})

	t.Run("When init container failed it should surface the error", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				InitContainerStatuses: []corev1.ContainerStatus{
					{
						Name: mustGatherInitContainer,
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 2, Reason: "Error"},
						},
					},
				},
			},
		}
		if mustGatherInitContainerSucceeded(pod) {
			t.Fatal("expected init container not succeeded")
		}
		if err := mustGatherInitContainerFailure(pod); err == nil {
			t.Fatal("expected init container failure")
		}
	})
}

// testRESTConfig is a minimal rest.Config for validation tests.
var testRESTConfig = &rest.Config{Host: "https://localhost:6443"}

func TestParseGatherTargets(t *testing.T) {
	t.Run("When empty it should return error", func(t *testing.T) {
		if _, err := parseGatherTargets(""); err == nil {
			t.Fatal("expected error for empty gather")
		}
	})

	t.Run("When osdctl legacy alias sc is used it should error", func(t *testing.T) {
		if _, err := parseGatherTargets("hcp,sc"); err == nil {
			t.Fatal("expected error for legacy gather target sc — use rc on RC ZOA")
		}
	})

	t.Run("When comma-separated valid targets it should parse", func(t *testing.T) {
		targets, err := parseGatherTargets("hcp,mc,rc")
		if err != nil {
			t.Fatal(err)
		}
		if len(targets) != 3 || targets[0] != "hcp" || targets[1] != "mc" || targets[2] != "rc" {
			t.Fatalf("expected hcp+mc+rc, got %v", targets)
		}
	})

	t.Run("When invalid target it should error", func(t *testing.T) {
		if _, err := parseGatherTargets("foo"); err == nil {
			t.Fatal("expected error for invalid gather target")
		}
	})
}

func TestResolveClusterIdentity(t *testing.T) {
	t.Run("When cluster_id provided it should derive Seaplane namespace from HostedCluster lookup", func(t *testing.T) {
		params := &ExecutionParams{
			Params:        map[string]string{"cluster_id": "1600392f-9a94-4957-b672-eff8dc2be0bb"},
			DynamicClient: mustGatherHostedClusterClient("sergio", "cluster-1600392f-9a94-4957-b672-eff8dc2be0bb"),
		}
		id, err := resolveClusterIdentity(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		if id.HCNamespace != "cluster-1600392f-9a94-4957-b672-eff8dc2be0bb" {
			t.Errorf("unexpected HC namespace: %q", id.HCNamespace)
		}
		if id.CPNamespace != "cluster-1600392f-9a94-4957-b672-eff8dc2be0bb-sergio" {
			t.Errorf("unexpected CP namespace: %q", id.CPNamespace)
		}
	})

	t.Run("When cluster_id has cluster- prefix it should normalize", func(t *testing.T) {
		params := &ExecutionParams{
			Params:        map[string]string{"cluster_id": "cluster-abc123"},
			DynamicClient: mustGatherHostedClusterClient("demo", "cluster-abc123"),
		}
		id, err := resolveClusterIdentity(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		if id.HCNamespace != "cluster-abc123" {
			t.Errorf("unexpected HC namespace: %q", id.HCNamespace)
		}
	})
}

func TestResolveCollectionNamespaces(t *testing.T) {
	action := &mustGather{}
	id := clusterIdentity{
		HCNamespace:       "cluster-abc",
		HostedClusterName: "demo",
		CPNamespace:       "cluster-abc-demo",
	}

	t.Run("When gather is mc it should include MC platform namespaces", func(t *testing.T) {
		ns := action.resolveCollectionNamespaces([]string{"mc"}, id, nil)
		if len(ns) != len(mcPlatformNamespaces) {
			t.Fatalf("expected %d MC namespaces, got %d: %v", len(mcPlatformNamespaces), len(ns), ns)
		}
		for _, want := range []string{"karpenter", "cert-manager", "hypershift", "kube-applier"} {
			found := false
			for _, n := range ns {
				if n == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in %v", want, ns)
			}
		}
	})

	t.Run("When gather is rc it should include RC platform and EKS infra namespaces", func(t *testing.T) {
		ns := action.resolveCollectionNamespaces([]string{"rc"}, id, nil)
		if len(ns) != len(rcPlatformNamespaces) {
			t.Fatalf("expected %d RC namespaces, got %d: %v", len(rcPlatformNamespaces), len(ns), ns)
		}
		for _, want := range []string{"karpenter", "eks-nodepool", "platform-api", "seaplane", "grafana", "thanos-operator", "aws-load-balancer-controller"} {
			found := false
			for _, n := range ns {
				if n == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in %v", want, ns)
			}
		}
		for _, n := range ns {
			if n == "kube-applier" {
				t.Errorf("kube-applier is MC-only and should not be in RC gather: %v", ns)
			}
		}
	})

	t.Run("When gather is hcp it should include HC and CP namespaces", func(t *testing.T) {
		ns := action.resolveCollectionNamespaces([]string{"hcp"}, id, nil)
		if len(ns) != 5 {
			t.Fatalf("expected 5 namespaces, got %d: %v", len(ns), ns)
		}
	})
}

func TestScanGatherLogIssues(t *testing.T) {
	t.Run("When gather log contains RBAC errors it should count and sample them", func(t *testing.T) {
		log := "collecting resources\npods is forbidden: User cannot list pods\ncontinuing\n"
		count, samples := scanGatherLogIssues(log)
		if count != 1 {
			t.Fatalf("expected 1 issue, got %d", count)
		}
		if len(samples) != 1 || !strings.Contains(samples[0], "forbidden") {
			t.Fatalf("unexpected samples: %v", samples)
		}
	})
}

func TestMustGatherValidateGatherTargets(t *testing.T) {
	action := &mustGather{}

	t.Run("When gather is rc only it should not require cluster_id", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"gather": "rc", "skip_must_gather_image": "true"},
			TargetType: "rc",
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("When gather is mc only it should not require cluster_id", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"gather": "mc", "skip_must_gather_image": "true"},
			TargetType: "mc",
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("When gather includes hcp without cluster_id it should fail", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"gather": "hcp"},
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing cluster_id")
		}
	})

	t.Run("When cluster_id resolves HostedCluster it should pass", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{
				"gather":                 "hcp",
				"cluster_id":             "abc123",
				"skip_must_gather_image": "true",
			},
			TargetType:    "mc",
			KubeClient:    fake.NewClientset(),
			DynamicClient: mustGatherHostedClusterClient("demo", "cluster-abc123"),
			Logger:        mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("When cluster_id without resolvable HostedCluster it should fail", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{
				"gather":                 "hcp",
				"cluster_id":             "abc123",
				"skip_must_gather_image": "true",
			},
			TargetType: "mc",
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error when HostedCluster cannot be resolved")
		}
	})

	t.Run("When gather rc on mc deployment it should fail", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"gather": "rc"},
			TargetType: "mc",
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for gather=rc on mc endpoint")
		}
	})

	t.Run("When gather hcp on rc deployment it should fail", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{
				"gather":     "hcp",
				"cluster_id": "abc123",
			},
			TargetType: "rc",
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for gather=hcp on rc endpoint")
		}
	})

	t.Run("When gather is missing it should fail", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{},
			KubeClient: fake.NewClientset(),
			Logger:     mustGatherTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing gather")
		}
	})
}
