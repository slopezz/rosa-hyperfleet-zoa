//go:build e2e

// CLI: zoa breakglass — scope catalog and API wiring (ROSAENG-75954).

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var expectedBreakglassScopeIDs = []string{
	"aws-elevated",
	"aws-read",
	"aws-write",
	"kube-elevated",
	"kube-read",
	"kube-write",
}

var _ = Describe("zoa breakglass", func() {
	It("lists embedded scope catalog without API", Label("smoke"), func() {
		cmd := exec.Command(zoaBin, "breakglass", "scopes", "--offline", "-o", "json") //nolint:gosec // test-controlled
		cmd.Env = filterEnv(os.Environ(), "ZOA_API_URL", "AWS_PROFILE")
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))

		var list struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Count int `json:"count"`
		}
		Expect(json.Unmarshal(extractJSON(string(out)), &list)).To(Succeed(), string(out))
		Expect(list.Count).To(Equal(len(expectedBreakglassScopeIDs)))

		var ids []string
		for _, item := range list.Items {
			ids = append(ids, item.ID)
		}
		sort.Strings(ids)
		Expect(ids).To(Equal(sortedCopy(expectedBreakglassScopeIDs)))
	})

	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			It("lists break-glass scopes from the API", Label("smoke"), func() {
				out, err := runZoa(tgt, "breakglass", "scopes", "-o", "json")
				Expect(err).NotTo(HaveOccurred(), out)

				var list struct {
					Items []struct {
						ID string `json:"id"`
					} `json:"items"`
				}
				Expect(json.Unmarshal([]byte(extractJSON(out)), &list)).To(Succeed(), out)
				Expect(list.Items).NotTo(BeEmpty())

				var ids []string
				for _, item := range list.Items {
					ids = append(ids, item.ID)
				}
				sort.Strings(ids)
				Expect(ids).To(Equal(sortedCopy(expectedBreakglassScopeIDs)))
			})
		})
	}
})

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
