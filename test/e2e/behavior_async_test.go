//go:build e2e

// Behavior: async execution — fire-and-forget dispatch, --wait polling, reconnect.

package e2e

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("async execution", func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			// Smoke test: One async execution with --wait to verify the async path
			// works end-to-end. Limited to one test because async executions wait
			// for EventBridge reconciler (~60s each) — keeping smoke fast (~2min).
			It("dispatches async and waits for completion", Label("smoke"), func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--execution-mode", "async",
					"--wait",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var exec struct {
					ID            string `json:"id"`
					Status        string `json:"status"`
					ExecutionMode string `json:"execution_mode"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec.ID).NotTo(BeEmpty())
				Expect(exec.Status).To(Equal("succeeded"))
				Expect(exec.ExecutionMode).To(Equal("async"))
			})

			It("dispatches async fire-and-forget (no --wait)", func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--execution-mode", "async",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var exec struct {
					ID            string `json:"id"`
					Status        string `json:"status"`
					ExecutionMode string `json:"execution_mode"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec.ID).NotTo(BeEmpty())
				Expect(exec.Status).To(Equal("dispatched"))
				Expect(exec.ExecutionMode).To(Equal("async"))

				// Eventually completes (reconciler picks it up)
				Eventually(func() string {
					getOut, getErr := runZoa(tgt, "get", exec.ID, "-o", "json")
					Expect(getErr).NotTo(HaveOccurred(), getOut)
					jsonOut := extractJSON(getOut)
					var result struct {
						Status string `json:"status"`
					}
					_ = json.Unmarshal([]byte(jsonOut), &result)
					return result.Status
				}, "90s", "5s").Should(Equal("succeeded"))
			})

			It("async write TA with --wait", func() {
				out, err := runZoa(tgt, "run", "rollout_restart",
					"--reason", reasonValue,
					"--namespace", coreDNSNamespace,
					"--resource", "deployment",
					"--name", coreDNSName,
					"--execution-mode", "async",
					"--force", // bypass cooldown
					"--wait",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var exec struct {
					ID     string `json:"id"`
					Status string `json:"status"`
					Action string `json:"action"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec.Status).To(Equal("succeeded"))
				Expect(exec.Action).To(Equal("rollout_restart"))
			})
		})
	}
})
