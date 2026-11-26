package actions

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "pods"}:            "PodList",
			{Group: "", Version: "v1", Resource: "services"}:        "ServiceList",
			{Group: "", Version: "v1", Resource: "namespaces"}:      "NamespaceList",
			{Group: "", Version: "v1", Resource: "configmaps"}:      "ConfigMapList",
			{Group: "", Version: "v1", Resource: "secrets"}:         "SecretList",
			{Group: "", Version: "v1", Resource: "nodes"}:           "NodeList",
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		},
		objects...,
	)
}

func getresourceTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func newFakeKubeClient() *fake.Clientset {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", ShortNames: []string{"po"}},
				{Name: "services", SingularName: "service", Namespaced: true, Kind: "Service", ShortNames: []string{"svc"}},
				{Name: "namespaces", SingularName: "namespace", Namespaced: false, Kind: "Namespace", ShortNames: []string{"ns"}},
				{Name: "nodes", SingularName: "node", Namespaced: false, Kind: "Node", ShortNames: []string{"no"}},
				{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap", ShortNames: []string{"cm"}},
				{Name: "secrets", SingularName: "secret", Namespaced: true, Kind: "Secret"},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", SingularName: "deployment", Namespaced: true, Kind: "Deployment", ShortNames: []string{"deploy"}},
			},
		},
	}
	return client
}

func fakePod(ns, name, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "main", "image": "nginx"},
				},
			},
			"status": map[string]interface{}{
				"phase": phase,
			},
		},
	}
}

func TestGetResource(t *testing.T) {
	action := &getResource{}

	t.Run("When resource param is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:        map[string]string{},
			DynamicClient: newFakeDynamicClient(),
			Logger:        getresourceTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing resource param")
		}
	})

	t.Run("When resource type is unknown it should pass validation (resolved via discovery at execution)", func(t *testing.T) {
		params := &ExecutionParams{
			Params:        map[string]string{"resource": "hostedclusters"},
			DynamicClient: newFakeDynamicClient(),
			Logger:        getresourceTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("expected unknown resource to pass validation, got: %v", err)
		}
	})

	t.Run("When requesting secrets it should redirect to get_secret action", func(t *testing.T) {
		params := &ExecutionParams{
			Params:        map[string]string{"resource": "secrets", "namespace": "default"},
			DynamicClient: newFakeDynamicClient(),
			Logger:        getresourceTestLogger(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected validation error for secrets resource")
		}
		if !strings.Contains(err.Error(), "get_secret") {
			t.Fatalf("expected error to mention get_secret, got: %v", err)
		}
	})

	t.Run("When requesting secret (singular) it should also redirect", func(t *testing.T) {
		params := &ExecutionParams{
			Params:        map[string]string{"resource": "secret"},
			DynamicClient: newFakeDynamicClient(),
			Logger:        getresourceTestLogger(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected validation error for secret resource")
		}
		if !strings.Contains(err.Error(), "get_secret") {
			t.Fatalf("expected error to mention get_secret, got: %v", err)
		}
	})

	t.Run("When getting a specific pod in verbose mode it should return full object", func(t *testing.T) {
		client := newFakeDynamicClient(
			fakePod("default", "my-pod", "Running"),
		)
		params := &ExecutionParams{
			Params:        map[string]string{"resource": "pods", "namespace": "default", "name": "my-pod", "verbose": "true"},
			DynamicClient: client,
			KubeClient:    newFakeKubeClient(),
			Logger:        getresourceTestLogger(),
		}

		result, err := action.Execute(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatal("expected success")
		}

		obj, ok := result.Output.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map[string]interface{}, got %T", result.Output)
		}
		if _, ok := obj["spec"]; !ok {
			t.Error("verbose output should contain 'spec' field")
		}
		if _, ok := obj["status"]; !ok {
			t.Error("verbose output should contain 'status' field")
		}
	})

	t.Run("When listing pods verbose it should return full objects", func(t *testing.T) {
		client := newFakeDynamicClient(
			fakePod("default", "pod-1", "Running"),
			fakePod("default", "pod-2", "Pending"),
		)
		params := &ExecutionParams{
			Params:        map[string]string{"resource": "pods", "namespace": "default", "verbose": "true"},
			DynamicClient: client,
			KubeClient:    newFakeKubeClient(),
			Logger:        getresourceTestLogger(),
		}

		result, err := action.Execute(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		items, ok := result.Output.([]map[string]interface{})
		if !ok {
			t.Fatalf("expected []map[string]interface{}, got %T", result.Output)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 pods, got %d", len(items))
		}
	})

	t.Run("When getting a non-existent pod it should return error", func(t *testing.T) {
		client := newFakeDynamicClient()
		params := &ExecutionParams{
			Params:        map[string]string{"resource": "pods", "namespace": "default", "name": "no-such-pod", "verbose": "true"},
			DynamicClient: client,
			KubeClient:    newFakeKubeClient(),
			Logger:        getresourceTestLogger(),
		}

		_, err := action.Execute(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for non-existent pod")
		}
	})

	t.Run("When dynamic client is nil it should fail validation", func(t *testing.T) {
		params := &ExecutionParams{
			Params: map[string]string{"resource": "pods"},
			Logger: getresourceTestLogger(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected validation error when DynamicClient is nil")
		}
	})
}

func TestParseTableResponse(t *testing.T) {
	t.Run("When response is a valid Table with multiple rows it should parse correctly", func(t *testing.T) {
		table := map[string]interface{}{
			"kind":       "Table",
			"apiVersion": "meta.k8s.io/v1",
			"columnDefinitions": []interface{}{
				map[string]interface{}{"name": "Name"},
				map[string]interface{}{"name": "Ready"},
				map[string]interface{}{"name": "Status"},
				map[string]interface{}{"name": "Age"},
			},
			"rows": []interface{}{
				map[string]interface{}{"cells": []interface{}{"nginx-1", "1/1", "Running", "2d"}},
				map[string]interface{}{"cells": []interface{}{"nginx-2", "0/1", "Pending", "5m"}},
			},
		}
		raw, _ := json.Marshal(table)

		rows, count, err := parseTableResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected 2 rows, got %d", count)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows in slice, got %d", len(rows))
		}
		if rows[0]["Name"] != "nginx-1" {
			t.Errorf("expected Name=nginx-1, got %v", rows[0]["Name"])
		}
		if rows[1]["Status"] != "Pending" {
			t.Errorf("expected Status=Pending, got %v", rows[1]["Status"])
		}
	})

	t.Run("When response has a single row it should return a slice with one element", func(t *testing.T) {
		table := map[string]interface{}{
			"kind":       "Table",
			"apiVersion": "meta.k8s.io/v1",
			"columnDefinitions": []interface{}{
				map[string]interface{}{"name": "Name"},
				map[string]interface{}{"name": "Status"},
			},
			"rows": []interface{}{
				map[string]interface{}{"cells": []interface{}{"my-pod", "Running"}},
			},
		}
		raw, _ := json.Marshal(table)

		rows, count, err := parseTableResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row, got %d", count)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 element in slice, got %d", len(rows))
		}
		if rows[0]["Name"] != "my-pod" {
			t.Errorf("expected Name=my-pod, got %v", rows[0]["Name"])
		}
	})

	t.Run("When response is not a Table kind it should return error", func(t *testing.T) {
		raw := []byte(`{"kind":"PodList","items":[]}`)
		_, _, err := parseTableResponse(raw)
		if err == nil {
			t.Fatal("expected error for non-Table kind")
		}
	})

	t.Run("When response is invalid JSON it should return error", func(t *testing.T) {
		_, _, err := parseTableResponse([]byte("not json"))
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("When table has zero rows it should return empty slice", func(t *testing.T) {
		table := map[string]interface{}{
			"kind":              "Table",
			"apiVersion":        "meta.k8s.io/v1",
			"columnDefinitions": []interface{}{map[string]interface{}{"name": "Name"}},
			"rows":              []interface{}{},
		}
		raw, _ := json.Marshal(table)

		rows, count, err := parseTableResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 rows, got %d", count)
		}
		if len(rows) != 0 {
			t.Errorf("expected empty slice, got %d items", len(rows))
		}
	})
}

func TestGetResourceMetadata(t *testing.T) {
	action := &getResource{}
	meta := action.Metadata()

	t.Run("When checking metadata it should have correct name and scope", func(t *testing.T) {
		if meta.Name != "get_resource" {
			t.Errorf("expected name 'get_resource', got %q", meta.Name)
		}
		if meta.Scope != "kube" {
			t.Errorf("expected scope 'kube', got %q", meta.Scope)
		}
		if meta.Type != "read" {
			t.Errorf("expected type 'read', got %q", meta.Type)
		}
	})

	t.Run("When checking metadata it should have resource parameter as required", func(t *testing.T) {
		found := false
		for _, p := range meta.Parameters {
			if p.Name == "resource" && p.Required {
				found = true
			}
		}
		if !found {
			t.Error("expected required 'resource' parameter")
		}
	})
}

func TestGetResourceRegistry(t *testing.T) {
	t.Run("When init runs it should register get_resource action", func(t *testing.T) {
		a, ok := Get("get_resource")
		if !ok {
			t.Fatal("get_resource action not registered")
		}
		if a.Metadata().Name != "get_resource" {
			t.Errorf("expected name 'get_resource', got %q", a.Metadata().Name)
		}
	})
}

func TestDiscoverGVR(t *testing.T) {
	t.Run("When discovery includes subresources it should skip them", func(t *testing.T) {
		client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
		client.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods/status", SingularName: "", Namespaced: true, Kind: "Pod"},
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", ShortNames: []string{"po"}},
					{Name: "pods/log", SingularName: "", Namespaced: true, Kind: "Pod"},
				},
			},
		}

		gvr, namespaced, err := discoverGVR(client.Discovery(), "pod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gvr.Resource != "pods" {
			t.Errorf("expected Resource=pods, got %q", gvr.Resource)
		}
		if !namespaced {
			t.Error("expected pods to be namespaced")
		}
	})

	t.Run("When input is plural it should match correctly", func(t *testing.T) {
		client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
		client.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", ShortNames: []string{"po"}},
				},
			},
		}

		gvr, _, err := discoverGVR(client.Discovery(), "pods")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gvr.Resource != "pods" {
			t.Errorf("expected Resource=pods, got %q", gvr.Resource)
		}
	})

	t.Run("When input is short name it should match", func(t *testing.T) {
		client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
		client.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", ShortNames: []string{"po"}},
				},
			},
		}

		gvr, _, err := discoverGVR(client.Discovery(), "po")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gvr.Resource != "pods" {
			t.Errorf("expected Resource=pods, got %q", gvr.Resource)
		}
	})

	t.Run("When same resource exists in core and extension groups it should prefer core", func(t *testing.T) {
		client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
		client.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "metrics.k8s.io/v1beta1",
				APIResources: []metav1.APIResource{
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "PodMetrics"},
				},
			},
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", ShortNames: []string{"po"}},
				},
			},
		}

		gvr, _, err := discoverGVR(client.Discovery(), "pod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gvr.Group != "" {
			t.Errorf("expected core group (empty), got %q", gvr.Group)
		}
		if gvr.Resource != "pods" {
			t.Errorf("expected Resource=pods, got %q", gvr.Resource)
		}
	})

	t.Run("When resource is not found it should return error", func(t *testing.T) {
		client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
		client.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"},
				},
			},
		}

		_, _, err := discoverGVR(client.Discovery(), "hostedclusters")
		if err == nil {
			t.Fatal("expected error for unknown resource")
		}
	})
}
