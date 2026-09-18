//go:build e2e_monitoring

// Package e2e_monitoring provides a minimal SigV4-signed HTTP client and
// Thanos query helpers for the ZOA monitoring E2E tests.  This is a
// self-contained copy of the patterns used in rosa-hyperfleet-api's
// test/helpers/{aws,thanos} — trimmed to only the functions we need so
// the ZOA repo has no cross-module dependency on the API repo.
package e2e_monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/onsi/ginkgo/v2"
)

const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// rhobsClient is a minimal SigV4-signing HTTP client for the RHOBS API
// Gateway.  It signs every request with execute-api / us-east-1.
type rhobsClient struct {
	baseURL    string
	region     string
	awsProfile string
	http       *http.Client
}

// newRHOBSClient creates a client for the given RHOBS API Gateway URL.
// awsProfile may be empty — in that case the default credential chain is used.
func newRHOBSClient(baseURL, region, awsProfile string) *rhobsClient {
	return &rhobsClient{
		baseURL:    baseURL,
		region:     region,
		awsProfile: awsProfile,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

type apiResponse struct {
	StatusCode int
	Body       []byte
}

func (c *rhobsClient) get(path string) (*apiResponse, error) {
	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var loadOpts []func(*config.LoadOptions) error
	loadOpts = append(loadOpts, config.WithRegion(c.region))
	if c.awsProfile != "" {
		loadOpts = append(loadOpts, config.WithSharedConfigProfile(c.awsProfile))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("retrieving credentials: %w", err)
	}

	signer := v4.NewSigner()
	if err := signer.SignHTTP(context.Background(), creds, req, emptyPayloadHash, "execute-api", c.region, time.Now()); err != nil {
		return nil, fmt.Errorf("signing request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return &apiResponse{StatusCode: resp.StatusCode, Body: body}, nil
}

// post sends a POST request with a JSON body (used only if needed in future).
func (c *rhobsClient) post(path string, payload []byte) (*apiResponse, error) {
	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var loadOpts []func(*config.LoadOptions) error
	loadOpts = append(loadOpts, config.WithRegion(c.region))
	if c.awsProfile != "" {
		loadOpts = append(loadOpts, config.WithSharedConfigProfile(c.awsProfile))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("retrieving credentials: %w", err)
	}

	payloadHash := sha256.Sum256(payload)
	signer := v4.NewSigner()
	if err := signer.SignHTTP(context.Background(), creds, req, hex.EncodeToString(payloadHash[:]), "execute-api", c.region, time.Now()); err != nil {
		return nil, fmt.Errorf("signing request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return &apiResponse{StatusCode: resp.StatusCode, Body: body}, nil
}

// ---------------------------------------------------------------------------
// Thanos query helpers
// ---------------------------------------------------------------------------

// QueryResponse represents the JSON response from /api/v1/query.
type QueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// RulesResponse represents the JSON response from /api/v1/rules.
type RulesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Groups []RuleGroup `json:"groups"`
	} `json:"data"`
}

// RuleGroup is a single group from the rules response.
type RuleGroup struct {
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// Rule is a single recording or alerting rule.
type Rule struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Query  string            `json:"query"`
	Labels map[string]string `json:"labels"`
	State  string            `json:"state"`
}

// thanosQuery executes a PromQL instant query and returns the parsed response.
func thanosQuery(client *rhobsClient, promql string) QueryResponse {
	path := fmt.Sprintf("/api/v1/query?query=%s", url.QueryEscape(promql))
	resp, err := client.get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Thanos query error: %v\n", err)
		return QueryResponse{}
	}
	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("Thanos query returned %d: %s\n", resp.StatusCode, string(resp.Body))
		return QueryResponse{}
	}
	var result QueryResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to parse Thanos response: %v\n", err)
		return QueryResponse{}
	}
	ginkgo.GinkgoWriter.Printf("Thanos query: %s → %d results\n", promql, len(result.Data.Result))
	return result
}

// thanosQueryRules fetches rules of the given type ("record" or "alert").
func thanosQueryRules(client *rhobsClient, ruleType string) RulesResponse {
	path := fmt.Sprintf("/api/v1/rules?type=%s", url.QueryEscape(ruleType))
	resp, err := client.get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Thanos rules query error: %v\n", err)
		return RulesResponse{}
	}
	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("Thanos rules query returned %d: %s\n", resp.StatusCode, string(resp.Body))
		return RulesResponse{}
	}
	var result RulesResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to parse Thanos rules response: %v\n", err)
		return RulesResponse{}
	}
	totalRules := 0
	for _, g := range result.Data.Groups {
		totalRules += len(g.Rules)
	}
	ginkgo.GinkgoWriter.Printf("Thanos rules query (type=%s) → %d groups, %d rules\n",
		ruleType, len(result.Data.Groups), totalRules)
	return result
}

// hasRule returns true if a rule with the given type and name is loaded.
func hasRule(client *rhobsClient, ruleType, ruleName string) bool {
	rules := thanosQueryRules(client, ruleType)
	for _, group := range rules.Data.Groups {
		for _, r := range group.Rules {
			if r.Name == ruleName {
				ginkgo.GinkgoWriter.Printf("Found rule %s (type=%s, state=%s)\n", r.Name, r.Type, r.State)
				return true
			}
		}
	}
	return false
}

// envOrDefault returns the environment variable value or a default.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
