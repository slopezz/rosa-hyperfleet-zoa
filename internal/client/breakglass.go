package client

import (
	"context"
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
	ID          string `json:"id"`
	Plane       string `json:"plane"`
	Type        string `json:"type"`
	Description string `json:"description"`
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

func (c *Client) ListBreakglassRequests(ctx context.Context) (*BreakglassRequestList, error) {
	var resp BreakglassRequestList
	if err := c.doV0(ctx, http.MethodGet, "/breakglass/requests", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ConnectBreakglass(ctx context.Context, id string, withCredentials bool) (*BreakglassConnectResponse, error) {
	path := "/breakglass/requests/" + url.PathEscape(id) + "/connect"
	if withCredentials {
		path += "?credentials=true"
	}
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
