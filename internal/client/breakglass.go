package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type BreakglassRequest struct {
	RequestID        string `json:"id"`
	Scope            string `json:"scope"`
	Reason           string `json:"reason"`
	Operator         string `json:"operator"`
	Status           string `json:"status"`
	TargetCluster    string `json:"target_cluster"`
	GrantExpiresAt   string `json:"grant_expires_at,omitempty"`
	RequestExpiresAt string `json:"request_expires_at,omitempty"`
	Kind             string `json:"kind,omitempty"`
}

type BreakglassScopeList struct {
	Items []ScopeMetadata `json:"items"`
	Count int             `json:"count"`
}

type ScopeMetadata struct {
	ID            string                 `json:"id"`
	Plane         string                 `json:"plane"`
	Type          string                 `json:"type"`
	Description   string                 `json:"description"`
	Authorization map[string]interface{} `json:"authorization,omitempty"`
	Capabilities  []string               `json:"capabilities,omitempty"`
	Restrictions  []string               `json:"restrictions,omitempty"`
}

type BreakglassRequestList struct {
	Items []BreakglassRequest `json:"items"`
	Count int                 `json:"count"`
}

type BreakglassConnectResponse struct {
	RequestID      string                 `json:"request_id"`
	Scope          string                 `json:"scope"`
	Status         string                 `json:"status"`
	GrantExpiresAt string                 `json:"grant_expires_at"`
	Kube           map[string]interface{} `json:"kube,omitempty"`
	AWS            map[string]interface{} `json:"aws,omitempty"`
	KubeToken      string                 `json:"kube_token,omitempty"`
}

func (c *Client) GetBreakglassScope(ctx context.Context, scopeID string) (*ScopeMetadata, error) {
	var resp ScopeMetadata
	if err := c.doV0(ctx, http.MethodGet, "/breakglass/scopes/"+url.PathEscape(scopeID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListBreakglassScopes(ctx context.Context) (*BreakglassScopeList, error) {
	var resp BreakglassScopeList
	if err := c.doV0(ctx, http.MethodGet, "/breakglass/scopes", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateBreakglassRequest(ctx context.Context, scope, reason string) (*BreakglassRequest, error) {
	body := map[string]string{"scope": scope, "reason": reason}
	var resp BreakglassRequest
	if err := c.doV0(ctx, http.MethodPost, "/breakglass/requests", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetBreakglassRequest(ctx context.Context, id string) (*BreakglassRequest, error) {
	var resp BreakglassRequest
	if err := c.doV0(ctx, http.MethodGet, "/breakglass/requests/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type BreakglassListParams struct {
	Scope    string // "mine" (default) or "fleet" for cross-operator history
	Operator string
	Status   string
	Since    string
	Until    string
	Limit    int
}

func (c *Client) ListBreakglassRequests(ctx context.Context, params *BreakglassListParams) (*BreakglassRequestList, error) {
	q := url.Values{}
	if params != nil {
		if params.Scope != "" {
			q.Set("scope", params.Scope)
		}
		if params.Operator != "" {
			q.Set("operator", params.Operator)
		}
		if params.Status != "" {
			q.Set("status", params.Status)
		}
		if params.Since != "" {
			q.Set("since", params.Since)
		}
		if params.Until != "" {
			q.Set("until", params.Until)
		}
		if params.Limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", params.Limit))
		}
	}
	path := "/breakglass/requests"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var resp BreakglassRequestList
	if err := c.doV0(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ActivateBreakglass provisions access and returns credentials (POST …/connect).
func (c *Client) ActivateBreakglass(ctx context.Context, id string) (*BreakglassConnectResponse, error) {
	path := "/breakglass/requests/" + url.PathEscape(id) + "/connect"
	var resp BreakglassConnectResponse
	if err := c.doV0(ctx, http.MethodPost, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Approve(ctx context.Context, compoundOrID, comment string) error {
	body := map[string]string{}
	if comment != "" {
		body["comment"] = comment
	}
	return c.doV0(ctx, http.MethodPost, "/approve/"+url.PathEscape(compoundOrID), body, nil)
}

func (c *Client) Reject(ctx context.Context, compoundOrID, comment string) error {
	body := map[string]string{}
	if comment != "" {
		body["comment"] = comment
	}
	return c.doV0(ctx, http.MethodPost, "/reject/"+url.PathEscape(compoundOrID), body, nil)
}
