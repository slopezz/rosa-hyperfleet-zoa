//go:build e2e

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Generic parameter validation tests — run once against a representative TA.
// The API validates params the same way for all TAs, so we don't need to
// repeat these tests for every action. TA-specific validation (e.g.,
// "unsupported resource type") should be tested in the TA's own test file.

var _ = Describe("parameter validation", func() {
	// Use first available target — validation logic is identical for RC and MC
	BeforeEach(func() {
		if len(targets) == 0 {
			Skip("no targets configured")
		}
	})

	// Use get_resource as the representative TA (has required params, common action)
	Describe("generic API validation", func() {
		It("rejects missing required parameter", func() {
			tgt := targets[0]
			// get_resource requires "resource" param
			out := runActionExpectFailure(tgt, "get_resource")
			Expect(out).To(SatisfyAny(
				ContainSubstring("required"),
				ContainSubstring("missing"),
			))
		})

		It("rejects unknown parameter", func() {
			tgt := targets[0]
			// Pass a param that doesn't exist in get_resource's metadata
			out := runActionExpectFailure(tgt, "get_resource",
				"--resource", "nodes",
				"--unknown-param", "value")
			Expect(out).To(SatisfyAny(
				ContainSubstring("unknown"),
				ContainSubstring("unrecognized"),
			))
		})

		It("rejects invalid reason format", func() {
			tgt := targets[0]
			// Run without the helper (which auto-adds valid reason)
			out, err := runZoa(tgt, "run", "get_resource",
				"--resource", "nodes",
				"--reason", "invalid-format")
			Expect(err).To(HaveOccurred())
			Expect(out).To(ContainSubstring("PROJECT-123"))
		})

		It("rejects action that does not exist", func() {
			tgt := targets[0]
			out := runActionExpectFailure(tgt, "nonexistent_action_e2e_test")
			Expect(out).To(SatisfyAny(
				ContainSubstring("not found"),
				ContainSubstring("unknown"),
			))
		})
	})
})
