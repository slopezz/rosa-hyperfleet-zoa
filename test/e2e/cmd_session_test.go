//go:build e2e

// Session lifecycle e2e (central profile): validation and --no-connect lifecycle without ECS Exec.
// Skipped unless sessionE2EEnabled() — TEMPORARY eph gate in helpers_session_test.go.

package e2e

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-online/rosa-seaplane-zoa/pkg/reason"
)

var _ = Describe("zoa session validation", func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, func() {
			var (
				deployment string
				targetID   string
			)

			BeforeEach(func() {
				skipUnlessSessionE2E()
				ctx, err := resolveSessionE2EContext(tgt.TargetType)
				Expect(err).NotTo(HaveOccurred())
				deployment = ctx.Deployment
				targetID = ctx.TargetID
			})

			It("rejects session start without --reason", Label("smoke"), func() {
				out, err := runZoaAccessExpectFailure(
					"session", "start", deployment, targetID,
					"--no-connect",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(SatisfyAny(
					ContainSubstring("reason"),
					ContainSubstring("required"),
				))
			})

			It("rejects session start with invalid reason", Label("smoke"), func() {
				out, err := runZoaAccessExpectFailure(
					"session", "start", deployment, targetID,
					"--reason", "invalid-format",
					"--no-connect",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(SatisfyAny(
					ContainSubstring(reason.ExampleJiraIssue),
					ContainSubstring("not valid"),
				))
			})

			It("rejects session start for unknown target", Label("smoke"), func() {
				out, err := runZoaAccessExpectFailure(
					"session", "start", deployment, "e2e-nonexistent-target-zoa",
					"--reason", reasonValue,
					"--no-connect",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(SatisfyAny(
					ContainSubstring("target_not_found"),
					ContainSubstring("not found"),
				))
			})
		})
	}
})

var _ = Describe("zoa session", Serial, func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, Ordered, func() {
			var (
				deployment      string
				targetID        string
				compoundID      string
				rawID           string
				sessionOperator string
				sessionReady    bool
			)

			BeforeAll(func() {
				if !sessionE2EEnabled() {
					return
				}
				ctx, err := resolveSessionE2EContext(tgt.TargetType)
				if err != nil {
					return
				}
				deployment = ctx.Deployment
				targetID = ctx.TargetID
				sessionReady = true
			})

			BeforeEach(func() {
				skipUnlessSessionE2E()
				if !sessionReady {
					Skip("session deployment not resolved")
				}
			})

			It("starts a boundary session without ECS Exec", Label("smoke"), func() {
				startOut, err := runZoaAccess(
					"session", "start", deployment, targetID,
					"--reason", reasonValue,
					"--no-connect", "-o", "json",
				)
				Expect(err).NotTo(HaveOccurred(), "zoa session start %q %q:\n%s", deployment, targetID, startOut)

				started, err := parseSessionStartJSON(startOut)
				Expect(err).NotTo(HaveOccurred(), startOut)
				Expect(started.SessionID).NotTo(BeEmpty())
				Expect(started.RawID).NotTo(BeEmpty())
				Expect(strings.ToLower(started.Status)).To(Equal("active"),
					"--no-connect should provision the boundary task without ECS Exec")
				compoundID = started.SessionID
				rawID = started.RawID
				Expect(validateSessionStartResponse(started, deployment, targetID, rawID)).NotTo(HaveOccurred())
			})

			It("lists the session for this target after start", Label("smoke"), func() {
				Expect(rawID).NotTo(BeEmpty(), "session start spec must run first in Ordered container")

				listOut, err := runZoaAccess(
					"session", "list", deployment,
					"-t", targetID,
					"--status", "active",
					"-o", "json",
				)
				Expect(err).NotTo(HaveOccurred(), listOut)

				rows, err := parseSessionListJSON(listOut)
				Expect(err).NotTo(HaveOccurred(), listOut)
				row := findSessionForTarget(rows, rawID, targetID)
				Expect(row).NotTo(BeNil(),
					"session %q for target %q not in mine=true list (same central profile as start): %v",
					rawID, targetID, rows)
				Expect(validateActiveSessionRecord(*row, targetID, rawID, reasonValue)).NotTo(HaveOccurred())
				sessionOperator = row.Operator
			})

			It("includes the session in history with full audit fields", Label("smoke"), func() {
				Expect(rawID).NotTo(BeEmpty())
				Expect(sessionOperator).NotTo(BeEmpty(), "list active spec must run first")

				histOut, err := runZoaAccess(
					"session", "history", deployment,
					"--since", "1h",
					"-t", targetID,
					"-o", "json",
				)
				Expect(err).NotTo(HaveOccurred(), histOut)

				rows, err := parseSessionListJSON(histOut)
				Expect(err).NotTo(HaveOccurred(), histOut)
				row := findSessionForTarget(rows, rawID, targetID)
				Expect(row).NotTo(BeNil(),
					"session %q for target %q not in history: %v", rawID, targetID, rows)
				Expect(validateActiveSessionRecord(*row, targetID, rawID, reasonValue)).NotTo(HaveOccurred())
				Expect(row.Operator).To(Equal(sessionOperator),
					"history operator should match list for the same session")
			})

			It("terminates the session", Label("smoke"), func() {
				Expect(compoundID).NotTo(BeEmpty())

				termOut, err := runZoaAccess("session", "terminate", compoundID)
				Expect(err).NotTo(HaveOccurred(), termOut)
			})

			It("lists the session as terminated for this target", Label("smoke"), func() {
				Expect(rawID).NotTo(BeEmpty())

				listOut, err := runZoaAccess(
					"session", "list", deployment,
					"-t", targetID,
					"--status", "terminated",
					"-o", "json",
				)
				Expect(err).NotTo(HaveOccurred(), listOut)

				rows, err := parseSessionListJSON(listOut)
				Expect(err).NotTo(HaveOccurred(), listOut)
				row := findSessionForTarget(rows, rawID, targetID)
				Expect(row).NotTo(BeNil(), "terminated session for target %q not in list: %v", targetID, rows)
				Expect(validateTerminatedSessionRecord(*row, targetID, rawID, reasonValue)).NotTo(HaveOccurred())
			})

			It("includes the terminated session in history with full audit fields", Label("smoke"), func() {
				Expect(rawID).NotTo(BeEmpty())
				Expect(sessionOperator).NotTo(BeEmpty(), "list active spec must run first")

				histOut, err := runZoaAccess(
					"session", "history", deployment,
					"--since", "1h",
					"-t", targetID,
					"-o", "json",
				)
				Expect(err).NotTo(HaveOccurred(), histOut)

				rows, err := parseSessionListJSON(histOut)
				Expect(err).NotTo(HaveOccurred(), histOut)
				row := findSessionForTarget(rows, rawID, targetID)
				Expect(row).NotTo(BeNil(),
					"terminated session %q for target %q not in history: %v", rawID, targetID, rows)
				Expect(validateTerminatedSessionRecord(*row, targetID, rawID, reasonValue)).NotTo(HaveOccurred())
				Expect(row.Operator).To(Equal(sessionOperator),
					"history operator should match list for the same session")
			})
		})
	}
})
