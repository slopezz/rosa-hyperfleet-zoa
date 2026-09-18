//go:build e2e_monitoring

package e2e_monitoring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Recording rules defined in rosa-hyperfleet alerting-rules/templates/zoa.yaml.
var zoaRecordingRules = []string{
	"zoa:ta_success_rate",
	"zoa:ta_total_executions",
	"zoa:api_availability",
	"zoa:lambda_error_rate",
	"zoa:reconciler_last_run",
	"zoa:gc_last_run",
	"zoa:reconciler_tick_count",
	"zoa:gc_tick_count",
}

var _ = Describe("ZOA Recording Rules", Label("smoke"), func() {
	for _, rule := range zoaRecordingRules {
		rule := rule // capture
		It("should have "+rule+" loaded in Thanos Ruler", func() {
			Eventually(func() bool {
				return hasRule(client, "record", rule)
			}, "5m", "15s").Should(BeTrue(),
				"Recording rule %s should be loaded in Thanos Ruler", rule)
		})
	}
})
