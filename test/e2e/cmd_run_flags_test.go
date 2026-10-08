//go:build e2e

// CLI: zoa run generic flags — --timeout, --no-wait, --verbose, --wait-* options.

package e2e

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Tests for zoa run flags that apply to any TA — not TA-specific behavior.
// Uses get_resource as the representative read TA (fast, safe, no cooldown).

var _ = Describe("zoa run flags", func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			It("--timeout overrides server-side execution timeout", func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--timeout", "60s",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var exec struct {
					Status string `json:"status"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec.Status).To(Equal("succeeded"))
			})

			It("--no-wait returns ID without fetching output", func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--no-wait")
				Expect(err).NotTo(HaveOccurred(), out)
				// --no-wait should still print the execution ID
				Expect(out).NotTo(BeEmpty())
				// Should NOT contain full JSON output since we skipped waiting
			})

			It("--verbose returns full JSON output from the action", func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--verbose",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var exec struct {
					Status string      `json:"status"`
					Output interface{} `json:"output"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec.Status).To(Equal("succeeded"))
			})

			It("--wait-timeout and --wait-poll-interval are accepted on sync", func() {
				// On sync these have no practical effect (sync returns inline),
				// but they should be accepted without error
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--wait-timeout", "30s",
					"--wait-poll-interval", "5s",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var exec struct {
					Status string `json:"status"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec.Status).To(Equal("succeeded"))
			})

			It("--timeout rejects values exceeding server max", func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"--timeout", "600s")
				Expect(err).To(HaveOccurred())
				Expect(out).To(ContainSubstring("timeout"))
			})

			It("-o json includes all dispatch response fields [smoke]", func() {
				out, err := runZoa(tgt, "run", "get_resource",
					"--reason", reasonValue,
					"--resource", "nodes",
					"-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var raw map[string]interface{}
				Expect(json.Unmarshal([]byte(jsonStr), &raw)).To(Succeed(), jsonStr)

				for _, field := range []string{
					"id", "action", "target_cluster", "operator",
					"status", "execution_mode", "scope", "type",
				} {
					val, ok := raw[field]
					Expect(ok).To(BeTrue(), "missing field %q in JSON output", field)
					Expect(val).NotTo(BeEmpty(), "field %q is empty in JSON output", field)
				}

				// Boolean fields must be present (they serialize even when false)
				for _, field := range []string{"dry_run", "force"} {
					_, ok := raw[field]
					Expect(ok).To(BeTrue(), "missing boolean field %q in JSON output", field)
				}

				// Numeric field: duration_ms must be present and positive for sync
				Expect(raw).To(HaveKey("duration_ms"))
				Expect(raw["duration_ms"]).To(BeNumerically(">", 0), "duration_ms should be positive for a completed execution")

				// Output must be present for a succeeded sync execution
				Expect(raw).To(HaveKey("output"))
				Expect(raw["output"]).NotTo(BeNil(), "output should be present for a succeeded sync execution")

				// Verify correct values for a read TA
				Expect(raw["action"]).To(Equal("get_resource"))
				Expect(raw["scope"]).To(Equal("kube-api"))
				Expect(raw["type"]).To(Equal("read"))
				Expect(raw["status"]).To(Equal("succeeded"))
				Expect(raw["execution_mode"]).To(Equal("sync"))
				Expect(raw["dry_run"]).To(BeFalse())
				Expect(raw["force"]).To(BeFalse())
			})
		})
	}
})
