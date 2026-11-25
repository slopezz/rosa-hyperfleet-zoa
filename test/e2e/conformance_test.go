//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E conformance — every TA on each live Lambda must have ta_* e2e coverage.
// Catalog may run ahead of the deployed image; ta_* specs skip missing actions.
// RC and MC registries differ when a TA declares TargetTypes for only one endpoint.

var _ = Describe("e2e conformance", func() {
	taTestFileContents := func() map[string]string {
		testDir := "."
		if _, err := os.Stat("test/e2e"); err == nil {
			testDir = "test/e2e"
		}

		taFiles, err := filepath.Glob(filepath.Join(testDir, "ta_*_test.go"))
		Expect(err).NotTo(HaveOccurred())

		contents := make(map[string]string, len(taFiles))
		for _, f := range taFiles {
			data, readErr := os.ReadFile(f)
			Expect(readErr).NotTo(HaveOccurred(), f)
			contents[filepath.Base(f)] = string(data)
		}
		return contents
	}

	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			It("every registered TA has a ta_* e2e test file", func() {
				taFileContents := taTestFileContents()

				for _, ta := range liveActionNames(tgt) {
					found := false
					for _, content := range taFileContents {
						if strings.Contains(content, ta) {
							found = true
							break
						}
					}
					Expect(found).To(BeTrue(),
						"TA %q is registered on %s Lambda but has no ta_*_test.go e2e coverage — "+
							"add a test file or add the TA name to an existing ta_* file", ta, tgt.TargetType)
				}
			})

		})
	}

	It("smoke tests cover both kube-api and aws-api scopes", func() {
		testDir := "."
		if _, err := os.Stat("test/e2e"); err == nil {
			testDir = "test/e2e"
		}

		// Union scopes across all configured targets (RC-only / MC-only TAs may
		// split scopes between endpoints).
		taScope := make(map[string]string)
		for _, tgt := range targets {
			for name, scope := range liveActionScopes(tgt) {
				taScope[name] = scope
			}
		}

		hasKubeAPISmoke := false
		hasAWSAPISmoke := false

		taFiles, err := filepath.Glob(filepath.Join(testDir, "ta_*_test.go"))
		Expect(err).NotTo(HaveOccurred())
		for _, f := range taFiles {
			data, readErr := os.ReadFile(f)
			Expect(readErr).NotTo(HaveOccurred(), f)
			content := string(data)
			if !strings.Contains(content, `Label("smoke")`) {
				continue
			}
			for ta, scope := range taScope {
				if strings.Contains(content, ta) {
					if scope == "kube" {
						hasKubeAPISmoke = true
					}
					if scope == "aws" {
						hasAWSAPISmoke = true
					}
				}
			}
		}

		Expect(hasKubeAPISmoke).To(BeTrue(), "smoke suite should include at least one kube-api TA")
		Expect(hasAWSAPISmoke).To(BeTrue(), "smoke suite should include at least one aws-api TA")
	})
})
