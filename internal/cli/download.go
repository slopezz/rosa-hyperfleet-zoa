package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

type downloadOptions struct {
	artifact string
	file     string
}

func newDownloadCommand(global *GlobalOptions) *cobra.Command {
	opts := &downloadOptions{}

	cmd := &cobra.Command{
		Use:   "download <execution-id> [flags]",
		Short: "Save execution artifacts to a local file",
		Long: `Save execution artifacts (output or logs) from S3 to a local file.

Streams directly from S3 via the ZOA API without buffering the entire payload
in memory — suitable for large artifacts like must-gather tarballs.

If -f is not specified, the file is saved as zoa-<execution-id>-<artifact>.json
in the current directory (prefixed for easy grouping in file listings).

To inspect output in the terminal, use 'zoa output' or 'zoa logs' instead.`,
		Example: `  # Save output (auto-named: zoa-<id>-output.json)
  zoa download abc-123-def

  # Save to a specific path
  zoa download abc-123-def -f vpc-info.json

  # Save logs instead of output
  zoa download abc-123-def --artifact logs`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return downloadArtifact(cmd.Context(), global, opts, args[0])
		},
	}

	cmd.Flags().StringVar(&opts.artifact, "artifact", "output", "Artifact to download: 'output' or 'logs'")
	cmd.Flags().StringVarP(&opts.file, "file", "f", "", "Destination file path (default: zoa-<id>-<artifact>.json)")

	return cmd
}

func resolveDownloadPath(opts *downloadOptions, executionID, contentType string) string {
	if opts.file != "" {
		return opts.file
	}
	ext := "json"
	if opts.artifact == "logs" {
		ext = "jsonl"
	} else if contentType == "application/gzip" {
		ext = "tar.gz"
	}
	return fmt.Sprintf("zoa-%s-%s.%s", executionID, opts.artifact, ext)
}

func streamToFile(url, outPath string) (int64, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(outPath)
	if err != nil {
		return 0, fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	return io.Copy(f, resp.Body)
}

func downloadArtifact(ctx context.Context, global *GlobalOptions, opts *downloadOptions, executionID string) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/trusted-actions/runs/%s/%s", executionID, opts.artifact)
	resp, err := c.RawGet(ctx, path)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("download failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	outPath := resolveDownloadPath(opts, executionID, contentType)

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("streaming artifact: %w", err)
	}

	// Only append newline for text-based formats (not binary archives)
	if contentType != "application/gzip" {
		_, _ = f.Write([]byte("\n"))
	}

	fmt.Fprintf(os.Stderr, "Saved %s (%d bytes) → %s\n", opts.artifact, n, outPath)
	return nil
}
