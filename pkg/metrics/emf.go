package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

const Namespace = "ZOA"

type MetricDefinition struct {
	Name string `json:"Name"`
	Unit string `json:"Unit"`
}

type emfMetadata struct {
	Timestamp         int64                `json:"Timestamp"`
	CloudWatchMetrics []emfMetricDirective `json:"CloudWatchMetrics"`
}

type emfMetricDirective struct {
	Namespace  string             `json:"Namespace"`
	Dimensions [][]string         `json:"Dimensions"`
	Metrics    []MetricDefinition `json:"Metrics"`
}

// Emit writes an EMF-formatted log line to stdout, which CloudWatch Logs
// automatically parses into CloudWatch Metrics. These metrics are then
// scraped by YACE into Prometheus for alerting via PrometheusRules.
func Emit(dimensions map[string]string, metrics map[string]MetricValue) {
	dimKeys := sortedKeys(dimensions)

	metricNames := sortedKeys(metrics)
	metricDefs := make([]MetricDefinition, 0, len(metricNames))
	for _, name := range metricNames {
		mv := metrics[name]
		metricDefs = append(metricDefs, MetricDefinition{
			Name: name,
			Unit: mv.Unit,
		})
	}

	payload := map[string]interface{}{
		"_aws": emfMetadata{
			Timestamp: time.Now().UnixMilli(),
			CloudWatchMetrics: []emfMetricDirective{{
				Namespace:  Namespace,
				Dimensions: [][]string{dimKeys},
				Metrics:    metricDefs,
			}},
		},
	}

	for k, v := range dimensions {
		payload[k] = v
	}
	for name, mv := range metrics {
		payload[name] = mv.Value
	}

	line, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stdout, string(line))
}

type MetricValue struct {
	Value interface{}
	Unit  string
}

func Count(v int) MetricValue {
	return MetricValue{Value: v, Unit: "Count"}
}

func Milliseconds(v int64) MetricValue {
	return MetricValue{Value: v, Unit: "Milliseconds"}
}

func Seconds(v float64) MetricValue {
	return MetricValue{Value: v, Unit: "Seconds"}
}

func Bytes(v int64) MetricValue {
	return MetricValue{Value: v, Unit: "Bytes"}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
