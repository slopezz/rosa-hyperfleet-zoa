package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAutoName_WhenOutputArtifact_ItShouldUseZoaPrefixAndJsonExtension(t *testing.T) {
	opts := &downloadOptions{artifact: "output", file: ""}
	expected := "zoa-exec-123-output.json"

	result := resolveDownloadPath(opts, "exec-123", "application/json")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestDownloadAutoName_WhenLogsArtifact_ItShouldUseZoaPrefixAndJsonlExtension(t *testing.T) {
	opts := &downloadOptions{artifact: "logs", file: ""}
	expected := "zoa-exec-456-logs.jsonl"

	result := resolveDownloadPath(opts, "exec-456", "text/plain")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestDownloadAutoName_WhenFileSpecified_ItShouldUseProvidedPath(t *testing.T) {
	opts := &downloadOptions{artifact: "output", file: "/tmp/custom.json"}
	expected := "/tmp/custom.json"

	result := resolveDownloadPath(opts, "exec-789", "application/json")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestDownloadAutoName_WhenGzipContentType_ItShouldUseTarGzExtension(t *testing.T) {
	opts := &downloadOptions{artifact: "output", file: ""}
	expected := "zoa-exec-mg1-output.tar.gz"

	result := resolveDownloadPath(opts, "exec-mg1", "application/gzip")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestDownloadStreamToFile_WhenServerReturns200_ItShouldSaveContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"result":"test-data"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "test-output.json")

	n, err := streamToFile(server.URL+"/artifact", outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == 0 {
		t.Error("expected non-zero bytes written")
	}

	content, _ := os.ReadFile(outPath)
	if string(content) != `{"result":"test-data"}` {
		t.Errorf("expected %q, got %q", `{"result":"test-data"}`, string(content))
	}
}

func TestDownloadStreamToFile_WhenServerReturns404_ItShouldReturnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "should-not-exist.json")

	_, err := streamToFile(server.URL+"/artifact", outPath)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
