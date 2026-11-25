//go:build e2e

// CLI: zoa runs — list executions with filters, output formats, and pagination.

package e2e

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("zoa runs", func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			It("lists recent executions in table format", func() {
				// Ensure at least one execution exists
				runAction(tgt, "get_resource", "--resource", "nodes")

				out, err := runZoa(tgt, "runs", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)
				Expect(out).To(ContainSubstring("ID"))
				Expect(out).To(ContainSubstring("ACTION"))
				Expect(out).To(ContainSubstring("STATUS"))
			})

			It("lists executions in wide format with all columns", func() {
				out, err := runZoa(tgt, "runs", "-o", "wide", "--limit", "5")
				Expect(err).NotTo(HaveOccurred(), out)
				// Wide format includes timestamp columns
				Expect(out).To(SatisfyAny(
					ContainSubstring("STARTED"),
					ContainSubstring("CREATED_AT"),
				))
				Expect(out).To(SatisfyAny(
					ContainSubstring("DURATION"),
					ContainSubstring("DUR"),
				))
			})

			It("lists executions in JSON format", func() {
				out, err := runZoa(tgt, "runs", "-o", "json", "--limit", "5")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						ID     string `json:"id"`
						Action string `json:"action"`
						Status string `json:"status"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
			})

			It("filters by action name", func() {
				// Run a known action first
				runAction(tgt, "get_resource", "--resource", "nodes")

				out, err := runZoa(tgt, "runs", "-o", "json", "--action", "get_resource", "--since", "10m", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						Action string `json:"action"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				Expect(list.Items).NotTo(BeEmpty())
				for _, e := range list.Items {
					Expect(e.Action).To(Equal("get_resource"))
				}
			})

			It("filters by scope (kube-api)", func() {
				out, err := runZoa(tgt, "runs", "-o", "json", "--scope", "kube", "--since", "1h", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						Scope string `json:"scope"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				for _, e := range list.Items {
					Expect(e.Scope).To(Equal("kube"))
				}
			})

			It("filters by type (read)", func() {
				out, err := runZoa(tgt, "runs", "-o", "json", "--type", "read", "--since", "1h", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						Type string `json:"type"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				for _, e := range list.Items {
					Expect(e.Type).To(Equal("read"))
				}
			})

			It("filters by status", func() {
				out, err := runZoa(tgt, "runs", "-o", "json", "--status", "succeeded", "--since", "1h", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						Status string `json:"status"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				for _, e := range list.Items {
					Expect(e.Status).To(Equal("succeeded"))
				}
			})

			It("filters by execution mode (sync)", func() {
				out, err := runZoa(tgt, "runs", "-o", "json", "--execution-mode", "sync", "--since", "1h", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						ExecutionMode string `json:"execution_mode"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				for _, e := range list.Items {
					Expect(e.ExecutionMode).To(Equal("sync"))
				}
			})

			It("filters forced executions", func() {
				out, err := runZoa(tgt, "runs", "-o", "json", "--force", "--since", "1h", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						Force bool `json:"force"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				for _, e := range list.Items {
					Expect(e.Force).To(BeTrue())
				}
			})

			It("filters with --until time window", func() {
				// --since 1h --until now should return recent results
				out, err := runZoa(tgt, "runs", "-o", "json", "--since", "1h", "--until", "0s", "--limit", "5")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []interface{} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
			})

			It("filters dry-run executions", func() {
				// Ensure a dry-run execution exists
				runAction(tgt, "rollout_restart",
					"--resource", "deployment", "--namespace", coreDNSNamespace, "--name", coreDNSName, "--dry-run")

				out, err := runZoa(tgt, "runs", "-o", "json", "--dry-run", "--since", "10m", "--limit", "10")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						DryRun bool `json:"dry_run"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)
				Expect(list.Items).NotTo(BeEmpty())
				for _, e := range list.Items {
					Expect(e.DryRun).To(BeTrue())
				}
			})
		})
	}
})
