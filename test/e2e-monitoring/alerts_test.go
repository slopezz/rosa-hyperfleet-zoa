//go:build e2e_monitoring

package e2e_monitoring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Alerting rules defined in rosa-hyperfleet alerting-rules/templates/zoa.yaml.
var zoaAlertingRules = []string{
	"ZOAExecutionsTotalFailure",
	"ZOAReconcilerStalled",
	"ZOADLQGrowing",
	"ZOADynamoDBThrottled",
	"ZOALambdaErrorRateHigh",
	"ZOAApiAvailabilityBelowSLO",
	"ZOATASuccessRateBelowSLO",
	"ZOALambdaThrottled",
	"ZOAGCStalled",
	"ZOAGCErrors",
	"ZOACircuitBreakerFlapping",
}

var _ = Describe("ZOA Alerting Rules", Label("smoke"), func() {
	for _, alert := range zoaAlertingRules {
		alert := alert // capture
		It("should have "+alert+" loaded in Thanos Ruler", func() {
			Eventually(func() bool {
				return hasRule(client, "alert", alert)
			}, "5m", "15s").Should(BeTrue(),
				"Alert %s should be loaded in Thanos Ruler", alert)
		})
	}
})
