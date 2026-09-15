package metrics

import "testing"

func TestNormalizeRoute_WhenKnownPaths_ItShouldReturnStableTemplates(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{"GET", "/health", "GET /health"},
		{"GET", "/health/", "GET /health"},
		{"GET", "/version", "GET /version"},
		{"GET", "/api/v0/trusted-actions", "GET /api/v0/trusted-actions"},
		{"GET", "/api/v0/trusted-actions/audit", "GET /api/v0/trusted-actions/audit"},
		{"GET", "/api/v0/trusted-actions/runs", "GET /api/v0/trusted-actions/runs"},
		{"POST", "/api/v0/trusted-actions/must_gather/run", "POST /api/v0/trusted-actions/{action}/run"},
		{"GET", "/api/v0/trusted-actions/get_resource", "GET /api/v0/trusted-actions/{action}"},
		{"GET", "/api/v0/trusted-actions/runs/abc-123", "GET /api/v0/trusted-actions/runs/{id}"},
		{"GET", "/api/v0/trusted-actions/runs/550e8400-e29b-41d4-a716-446655440000/output", "GET /api/v0/trusted-actions/runs/{id}/output"},
		{"GET", "/api/v0/trusted-actions/runs/550e8400-e29b-41d4-a716-446655440000/logs", "GET /api/v0/trusted-actions/runs/{id}/logs"},
		{"GET", "/other/550e8400-e29b-41d4-a716-446655440000", "GET /other/{id}"},
		{"GET", "/", "GET /"},
	}
	for _, tc := range cases {
		got := NormalizeRoute(tc.method, tc.path)
		if got != tc.want {
			t.Fatalf("NormalizeRoute(%q, %q): expected %q, got %q", tc.method, tc.path, tc.want, got)
		}
	}
}

func TestStatusClass_WhenStatusCodes_ItShouldMapToClasses(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{302, "3xx"},
		{404, "4xx"},
		{429, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
		{0, "other"},
		{99, "other"},
	}
	for _, tc := range cases {
		if got := StatusClass(tc.code); got != tc.want {
			t.Fatalf("status %d: expected %q, got %q", tc.code, tc.want, got)
		}
	}
}
