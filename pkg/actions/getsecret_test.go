package actions

import (
	"context"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetSecret(t *testing.T) {
	action := &getSecretAction{}

	t.Run("When namespace has cluster- prefix it should reject", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"namespace": "cluster-abc123"},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for HCP namespace")
		}
		if got := err.Error(); got != "access to secrets in namespace \"cluster-abc123\" is blocked: HCP namespace protection" {
			t.Fatalf("unexpected error: %s", got)
		}
	})

	t.Run("When namespace has cluster- CP suffix it should reject", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"namespace": "cluster-abc123-my-hc"},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for HCP control plane namespace")
		}
	})

	t.Run("When secret name contains kubeconfig it should reject", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"namespace": "cert-manager", "name": "admin-kubeconfig"},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for sensitive secret name")
		}
	})

	t.Run("When secret name contains pull-secret it should reject", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"namespace": "openshift-config", "name": "pull-secret"},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for pull-secret")
		}
	})

	t.Run("When secret name contains etcd-encryption it should reject", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"namespace": "kube-system", "name": "etcd-encryption-key"},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for etcd-encryption secret")
		}
	})

	t.Run("When namespace is missing it should reject", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for missing namespace")
		}
	})

	t.Run("When namespace is allowed it should pass validation", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"namespace": "cert-manager"},
			Logger: slog.Default(),
		}
		err := action.Validate(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("When fetching a specific secret it should return keys only by default", func(t *testing.T) {
		client := fake.NewSimpleClientset(&corev1.Secret{ //nolint:staticcheck // NewClientset requires generated apply configs
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-cert",
				Namespace: "cert-manager",
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": []byte("cert-data"),
				"tls.key": []byte("key-data"),
			},
		})

		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "cert-manager", "name": "tls-cert"},
			KubeClient: client,
			Logger:     slog.Default(),
		}

		result, err := action.Execute(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatal("expected success")
		}

		output, ok := result.Output.(map[string]interface{})
		if !ok {
			t.Fatal("expected map output")
		}
		if _, hasData := output["data"]; hasData {
			t.Fatal("should not have data field in non-verbose mode")
		}
		keys, ok := output["keys"].([]string)
		if !ok {
			t.Fatal("expected keys field")
		}
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d", len(keys))
		}
	})

	t.Run("When verbose is true it should return base64 data values", func(t *testing.T) {
		client := fake.NewSimpleClientset(&corev1.Secret{ //nolint:staticcheck // NewClientset requires generated apply configs
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret",
				Namespace: "default",
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"password": []byte("s3cr3t"),
			},
		})

		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default", "name": "my-secret", "verbose": "true"},
			KubeClient: client,
			Logger:     slog.Default(),
		}

		result, err := action.Execute(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output, ok := result.Output.(map[string]interface{})
		if !ok {
			t.Fatal("expected map output")
		}
		data, ok := output["data"].(map[string]string)
		if !ok {
			t.Fatal("expected data field in verbose mode")
		}
		if data["password"] != "s3cr3t" {
			t.Fatalf("expected password value, got %q", data["password"])
		}
	})

	t.Run("When listing secrets it should filter blocked names", func(t *testing.T) {
		client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "myapp"},
				Data:       map[string][]byte{"key": []byte("val")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "kubeconfig-admin", Namespace: "myapp"},
				Data:       map[string][]byte{"config": []byte("yaml")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "db-password", Namespace: "myapp"},
				Data:       map[string][]byte{"pass": []byte("pw")},
			},
		)

		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "myapp"},
			KubeClient: client,
			Logger:     slog.Default(),
		}

		result, err := action.Execute(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output, ok := result.Output.([]map[string]interface{})
		if !ok {
			t.Fatal("expected slice output for list")
		}
		if len(output) != 2 {
			t.Fatalf("expected 2 secrets (kubeconfig-admin filtered), got %d", len(output))
		}
		for _, s := range output {
			name := s["name"].(string)
			if name == "kubeconfig-admin" {
				t.Fatal("kubeconfig-admin should have been filtered")
			}
		}
	})

	t.Run("When fetching secret with kubeconfig in name via Execute it should reject", func(t *testing.T) {
		client := fake.NewSimpleClientset(&corev1.Secret{ //nolint:staticcheck // NewClientset requires generated apply configs
			ObjectMeta: metav1.ObjectMeta{Name: "admin-kubeconfig", Namespace: "default"},
			Data:       map[string][]byte{"config": []byte("data")},
		})

		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default", "name": "admin-kubeconfig"},
			KubeClient: client,
			Logger:     slog.Default(),
		}

		_, err := action.Execute(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for blocked secret name in Execute")
		}
	})

	t.Run("When metadata returns it should have correct action name", func(t *testing.T) {
		meta := action.Metadata()
		if meta.Name != "get_secret" {
			t.Fatalf("expected name get_secret, got %s", meta.Name)
		}
		if meta.Scope != "kube" {
			t.Fatalf("expected scope kube, got %s", meta.Scope)
		}
		if meta.Type != "read" {
			t.Fatalf("expected type read, got %s", meta.Type)
		}
	})
}
