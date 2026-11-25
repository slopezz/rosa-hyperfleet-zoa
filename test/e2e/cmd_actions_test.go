//go:build e2e

// CLI: zoa actions, zoa describe — list and inspect registered Trusted Actions.

package e2e

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("zoa actions", func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			It("lists live Trusted Actions that are declared in the catalog", Label("smoke"), func() {
				out, err := runZoa(tgt, "actions", "-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var list struct {
					Items []struct {
						Name string `json:"name"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &list)).To(Succeed(), out)

				var names []string
				for _, a := range list.Items {
					names = append(names, a.Name)
				}
				expectLiveActionsInCatalog(tgt, names)
			})

			It("lists actions in table format by default", func() {
				out, err := runZoa(tgt, "actions")
				Expect(err).NotTo(HaveOccurred(), out)
				Expect(out).To(ContainSubstring("NAME"))
				Expect(out).To(ContainSubstring("get_resource"))
			})
		})
	}
})

var _ = Describe("zoa describe", func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			It("describes an action with its parameters in JSON", func() {
				out, err := runZoa(tgt, "describe", "get_resource", "-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var action struct {
					Name       string `json:"name"`
					Parameters []struct {
						Name string `json:"name"`
					} `json:"parameters"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &action)).To(Succeed(), out)
				Expect(action.Name).To(Equal("get_resource"))

				var paramNames []string
				for _, p := range action.Parameters {
					paramNames = append(paramNames, p.Name)
				}
				Expect(paramNames).To(ContainElement("resource"))
			})

			It("describes a write action with cooldown info", func() {
				out, err := runZoa(tgt, "describe", "rollout_restart", "-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var action struct {
					Name                 string `json:"name"`
					Type                 string `json:"type"`
					WriteCooldownSeconds int    `json:"write_cooldown_seconds"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &action)).To(Succeed(), out)
				Expect(action.Name).To(Equal("rollout_restart"))
				Expect(action.Type).To(Equal("write"))
				Expect(action.WriteCooldownSeconds).To(BeNumerically(">", 0))
			})

			It("describes an AWS-scoped action", func() {
				out, err := runZoa(tgt, "describe", "list_eks_clusters", "-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				jsonStr := extractJSON(out)
				var action struct {
					Name  string `json:"name"`
					Scope string `json:"scope"`
				}
				Expect(json.Unmarshal([]byte(jsonStr), &action)).To(Succeed(), out)
				Expect(action.Name).To(Equal("list_eks_clusters"))
				Expect(action.Scope).To(Equal("aws"))
			})

			It("rejects an unknown action name", func() {
				out, err := runZoa(tgt, "describe", "nonexistent_action")
				Expect(err).To(HaveOccurred())
				Expect(out).To(ContainSubstring("not found"))
			})
		})
	}
})
