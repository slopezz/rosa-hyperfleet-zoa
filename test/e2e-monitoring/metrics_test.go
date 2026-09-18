//go:build e2e_monitoring

package e2e_monitoring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ZOA Metrics", func() {

	// Infrastructure metrics are always-on — they do not depend on TA
	// executions. Suitable for smoke.
	Context("infrastructure metrics", Label("smoke"), func() {

		It("should have Lambda invocation metrics for ZOA functions", func() {
			query := `count(aws_lambda_invocations_sum{dimension_FunctionName=~".*-zoa-(api|worker)"}) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected aws_lambda_invocations_sum for ZOA Lambda functions in Thanos "+
					"(CloudWatch → YACE → Prometheus → Thanos)")
		})

		It("should have reconciler tick metrics", func() {
			query := `count(aws_zoa_reconciler_last_run_maximum) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected aws_zoa_reconciler_last_run_maximum in Thanos "+
					"(EMF ReconcilerLastRun → CloudWatch → YACE → Prometheus → Thanos)")
		})
	})

	// Execution metrics require TA runs to produce data. These are populated
	// by the ZOA E2E smoke/full suite that runs before this monitoring suite.
	Context("execution metrics", func() {

		It("should have execution count metrics with expected dimensions", func() {
			query := `count(aws_zoa_execution_count_sum) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected aws_zoa_execution_count_sum in Thanos after TA executions")
		})

		It("should have execution metrics with Status dimension", func() {
			query := `count(aws_zoa_execution_count_sum{dimension_Status="succeeded"}) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected dimension_Status=succeeded on execution metrics")
		})

		It("should have execution metrics with Mode dimension", func() {
			query := `count(aws_zoa_execution_count_sum{dimension_Mode=~"sync|async"}) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected dimension_Mode (sync or async) on execution metrics")
		})

		It("should have HTTP request metrics", func() {
			query := `count(aws_zoa_http_request_count_sum) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected aws_zoa_http_request_count_sum in Thanos after API calls")
		})
	})

	// Recording rule values require both metrics and rules to be active.
	Context("recording rule values", func() {

		It("should have zoa:ta_success_rate producing results", func() {
			query := `count(zoa:ta_success_rate) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected zoa:ta_success_rate to produce results")
		})

		It("should have zoa:reconciler_last_run producing results", func() {
			query := `count(zoa:reconciler_last_run) > 0`
			Eventually(func() bool {
				resp := thanosQuery(client, query)
				return resp.Status == "success" && len(resp.Data.Result) > 0
			}, "5m", "15s").Should(BeTrue(),
				"Expected zoa:reconciler_last_run to produce results")
		})
	})
})
