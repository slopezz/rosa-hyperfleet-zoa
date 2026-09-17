package metrics

import (
	"regexp"
	"strings"
	"time"
)

var uuidSegment = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NormalizeRoute maps a raw HTTP path to a low-cardinality route template.
func NormalizeRoute(method, path string) string {
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}

	switch {
	case path == "/health":
		return method + " /health"
	case path == "/version":
		return method + " /version"
	case path == "/api/v0/trusted-actions":
		return method + " /api/v0/trusted-actions"
	case path == "/api/v0/trusted-actions/audit":
		return method + " /api/v0/trusted-actions/audit"
	case path == "/api/v0/trusted-actions/runs":
		return method + " /api/v0/trusted-actions/runs"
	case strings.HasPrefix(path, "/api/v0/trusted-actions/runs/"):
		rest := strings.TrimPrefix(path, "/api/v0/trusted-actions/runs/")
		parts := strings.Split(rest, "/")
		if len(parts) == 1 {
			return method + " /api/v0/trusted-actions/runs/{id}"
		}
		if len(parts) == 2 && (parts[1] == "output" || parts[1] == "logs") {
			return method + " /api/v0/trusted-actions/runs/{id}/" + parts[1]
		}
	case strings.HasPrefix(path, "/api/v0/trusted-actions/") && strings.HasSuffix(path, "/run"):
		action := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v0/trusted-actions/"), "/run")
		if action != "" && !strings.Contains(action, "/") {
			return method + " /api/v0/trusted-actions/{action}/run"
		}
	case strings.HasPrefix(path, "/api/v0/trusted-actions/"):
		action := strings.TrimPrefix(path, "/api/v0/trusted-actions/")
		if action != "" && !strings.Contains(action, "/") {
			return method + " /api/v0/trusted-actions/{action}"
		}
	}

	// Fallback: replace UUID-like segments to avoid cardinality explosions.
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if uuidSegment.MatchString(seg) {
			segments[i] = "{id}"
		}
	}
	return method + " " + strings.Join(segments, "/")
}

// StatusClass maps an HTTP status code to a coarse class for EMF dimensions.
func StatusClass(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500:
		return "5xx"
	default:
		return "other"
	}
}

// EmitHTTPRequest records a completed HTTP request (API Lambda only).
func EmitHTTPRequest(cluster, method, path string, statusCode int, durationMs int64) {
	routeTemplate := NormalizeRoute(method, path)
	dims := map[string]string{
		"Cluster":       cluster,
		"HandlerMode":   "api",
		"Method":        method,
		"RouteTemplate": routeTemplate,
		"StatusClass":   StatusClass(statusCode),
	}
	metrics := map[string]MetricValue{
		"HttpRequestCount":    Count(1),
		"HttpRequestDuration": Milliseconds(durationMs),
	}
	Emit(dims, metrics)
}

// Rejection reasons for RejectionCount metric.
const (
	RejectionWriteCooldown      = "write_cooldown"
	RejectionMaxConcurrent      = "max_concurrent"
	RejectionValidationFailed   = "validation_failed"
	RejectionCircuitBreakerOpen = "circuit_breaker_open"
	RejectionActionNotFound     = "action_not_found"
)

// EmitRejection records a request rejected before execution terminal state.
func EmitRejection(cluster, reason string) {
	Emit(
		map[string]string{
			"Cluster": cluster,
			"Reason":  reason,
		},
		map[string]MetricValue{
			"RejectionCount": Count(1),
		},
	)
}

// EmitExecution records a terminal execution transition.
func EmitExecution(cluster string, action, status, mode, scope, typ string, durationMs int64) {
	dims := map[string]string{
		"Cluster": cluster,
		"Action":  action,
		"Status":  status,
		"Mode":    mode,
		"Scope":   scope,
		"Type":    typ,
	}
	metrics := map[string]MetricValue{
		"ExecutionCount": Count(1),
	}
	if durationMs >= 0 {
		metrics["ExecutionDuration"] = Milliseconds(durationMs)
	}
	Emit(dims, metrics)
}

// EmitGCCleaned records a resource cleaned by the GC pipeline.
func EmitGCCleaned(cluster, resourceType string) {
	Emit(
		map[string]string{
			"Cluster":      cluster,
			"ResourceType": resourceType,
		},
		map[string]MetricValue{
			"GCCleanedResources": Count(1),
		},
	)
}

// EmitCircuitBreakerStateChange records a circuit breaker state transition.
func EmitCircuitBreakerStateChange(cluster, state string) {
	Emit(
		map[string]string{
			"Cluster": cluster,
			"State":   state,
		},
		map[string]MetricValue{
			"CircuitBreakerStateChange": Count(1),
		},
	)
}

// EmitReconciler records one reconciler tick: duration, error count, and a
// unix-seconds last-run gauge used for "now - last tick" alerting.
func EmitReconciler(cluster string, durationMs int64, phaseErrors int) {
	Emit(
		map[string]string{
			"Cluster":     cluster,
			"HandlerMode": "reconciler",
		},
		map[string]MetricValue{
			"ReconcilerDuration": Milliseconds(durationMs),
			"ReconcilerErrors":   Count(phaseErrors),
			"ReconcilerLastRun":  Seconds(float64(time.Now().Unix())),
		},
	)
}

// EmitGC records one GC tick: duration, error count, and a unix-seconds
// last-run gauge — symmetric with EmitReconciler for tick-health monitoring.
func EmitGC(cluster string, durationMs int64, phaseErrors int) {
	Emit(
		map[string]string{
			"Cluster":     cluster,
			"HandlerMode": "gc",
		},
		map[string]MetricValue{
			"GCDuration": Milliseconds(durationMs),
			"GCErrors":   Count(phaseErrors),
			"GCLastRun":  Seconds(float64(time.Now().Unix())),
		},
	)
}
