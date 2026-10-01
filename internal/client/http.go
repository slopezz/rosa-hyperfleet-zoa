package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiV0Prefix = "/api/v0"

type requestConfig struct {
	urlPrefix   string
	maxBodySize int64
}

func defaultV0Request() requestConfig {
	return requestConfig{urlPrefix: apiV0Prefix, maxBodySize: 10 << 20}
}

func rootRequest() requestConfig {
	return requestConfig{urlPrefix: "", maxBodySize: 1 << 20}
}

// do performs a signed JSON request against the ZOA API.
// path is relative to /api/v0 unless requestConfig overrides urlPrefix (e.g. /version).
func (c *Client) do(ctx context.Context, method, path string, body any, result any, cfg requestConfig) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	fullURL := c.baseURL + cfg.urlPrefix + path

	var bodyReader io.ReadSeeker
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.accountID != "" {
		req.Header.Set("X-Account-ID", c.accountID)
	}
	if c.operator != "" {
		req.Header.Set("X-Operator", c.operator)
	}

	if c.signer != nil {
		creds, err := c.credentials.Retrieve(ctx)
		if err != nil {
			return fmt.Errorf("retrieving AWS credentials: %w", err)
		}
		payloadHash := sha256Hash(bodyReader)
		if err := c.signer.SignHTTP(ctx, creds, req, payloadHash, c.sigService, c.region, time.Now()); err != nil {
			return fmt.Errorf("signing request: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, cfg.maxBodySize))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	surface := surfaceForPath(cfg.urlPrefix + path)
	if err := decodeResponse(surface, resp.StatusCode, respBody, result); err != nil {
		return err
	}
	return nil
}

func (c *Client) doV0(ctx context.Context, method, path string, body any, result any) error {
	return c.do(ctx, method, path, body, result, defaultV0Request())
}

// surfaceForPath labels CLI errors; all v0 resources share one product surface name.
func surfaceForPath(fullPath string) APISurface {
	return APISurfaceZOA
}

func (c *Client) signRequest(ctx context.Context, req *http.Request, bodyReader io.ReadSeeker) error {
	if c.signer == nil {
		return nil
	}
	if c.accountID != "" {
		req.Header.Set("X-Account-ID", c.accountID)
	}
	if c.operator != "" {
		req.Header.Set("X-Operator", c.operator)
	}
	creds, err := c.credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("retrieving AWS credentials: %w", err)
	}
	payloadHash := sha256Hash(bodyReader)
	if err := c.signer.SignHTTP(ctx, creds, req, payloadHash, c.sigService, c.region, time.Now()); err != nil {
		return fmt.Errorf("signing request: %w", err)
	}
	return nil
}
