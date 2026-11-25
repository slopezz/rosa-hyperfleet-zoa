//go:build e2e

// Boundary exec-script: operator path inside the task (session start|join --exec-script).
// Skipped unless sessionE2EEnabled() — TEMPORARY eph gate in helpers_session_test.go.

package e2e

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-seaplane-zoa/internal/cli"
)

const boundaryExecOKMarker = "__ZOA_E2E_BOUNDARY_OK__"
const boundaryExecJSONMarker = "__ZOA_E2E_EXEC_JSON__:"

// boundaryExecScript is passed to session start --exec-script. Runs in /home/sre over ECS Exec.
func boundaryExecScript(reason, expectedDeployment, expectedTarget string) string {
	return fmt.Sprintf(`set -euo pipefail
expected_deployment=%q
expected_target=%q
test -f /home/sre/.claude/ZOA_SESSION.md
test -n "${ZOA_API_URL:-}"
command -v jq >/dev/null
command -v claude >/dev/null
test -n "${ZOA_DEPLOYMENT:-}"
test -n "${ZOA_TARGET:-}"
grep -Fq "${ZOA_DEPLOYMENT}" /home/sre/.claude/ZOA_SESSION.md
grep -Fq "${ZOA_TARGET}" /home/sre/.claude/ZOA_SESSION.md
grep -Fq %q /home/sre/.claude/ZOA_SESSION.md
reply=$(env -u ZOA_DEPLOYMENT -u ZOA_TARGET claude -p "Do you know where you are deployed? Answer in one short sentence: name the deployment and the target for this ZOA boundary session.")
grep -Fq "$expected_deployment" <<<"$reply"
grep -Fq "$expected_target" <<<"$reply"
zoa actions -o json | jq -e '.items | type == "array" and (map(select(.name == "get_resource")) | length) > 0'
exec_json=$(zoa run get_resource --resource nodes --reason %q -o json)
test -n "${ZOA_OPERATOR:-}"
test -n "${ZOA_SESSION_ID:-}"
echo "$exec_json" | jq -e --arg op "${ZOA_OPERATOR}" --arg sid "${ZOA_SESSION_ID}" '
  .status == "succeeded"
  and .action == "get_resource"
  and .scope == "kube"
  and (.operator | type == "string")
  and (.operator == $op)
  and (.session_id == $sid)
  and (.output | type == "array")
  and (.output | length > 0)'
echo %s$(echo "$exec_json" | jq -c .)
echo %s
`, expectedDeployment, expectedTarget, reason, reason, boundaryExecJSONMarker, boundaryExecOKMarker)
}

const boundaryExecReconnectScript = "set -euo pipefail\nzoa version"

// assertBoundaryExecScriptTranscript checks MOTD, two-line PS1, session limits, and exit hints in the exec-script PTY transcript
// (same audit surface as interactive Exec + CloudWatch ssm-sessions).
func assertBoundaryExecScriptTranscript(scriptOut, deployment, targetID, reason, compoundID, operator string) {
	Expect(operator).NotTo(BeEmpty(), "operator required for PS1 transcript assertions")
	Expect(scriptOut).To(ContainSubstring("Hello, "+operator), scriptOut)
	Expect(scriptOut).To(ContainSubstring("==> ZOA Boundary session"), scriptOut)
	Expect(scriptOut).To(ContainSubstring("Operator:"), scriptOut)
	Expect(scriptOut).To(ContainSubstring("Session:"), scriptOut)
	Expect(scriptOut).To(ContainSubstring("Deployment:"), scriptOut)
	Expect(scriptOut).To(ContainSubstring("Target:"), scriptOut)
	Expect(scriptOut).To(ContainSubstring(compoundID), scriptOut)
	Expect(scriptOut).To(ContainSubstring(deployment), scriptOut)
	Expect(scriptOut).To(ContainSubstring(targetID), scriptOut)
	Expect(scriptOut).To(ContainSubstring("==> Session reason"), scriptOut)
	Expect(scriptOut).To(ContainSubstring(reason), scriptOut)
	Expect(scriptOut).To(ContainSubstring("==> ZOA Boundary session limits"), scriptOut)
	Expect(scriptOut).To(ContainSubstring("Hard termination"), scriptOut)
	Expect(scriptOut).To(ContainSubstring("sessionId:"+compoundID),
		"expected primary PS1 sessionId line in exec-script transcript")
	Expect(scriptOut).To(ContainSubstring(operator+"@zoa:"+deployment+"/"+targetID),
		"expected secondary PS1 operator@zoa:deployment/target line in exec-script transcript")
	Expect(scriptOut).To(ContainSubstring("==> ZOA boundary — Exec ended (task still running)"),
		"expected boundary shell exit hint in exec-script transcript")
	Expect(scriptOut).To(ContainSubstring("zoa session terminate "+compoundID), scriptOut)
	Expect(scriptOut).To(ContainSubstring("==> ZOA boundary — Exec ended (ECS task may still be running)"),
		"expected laptop CLI exit hint after exec-script")
	Expect(scriptOut).To(ContainSubstring("Hard termination (UTC):"), scriptOut)
}

// assertBoundaryExecScriptOutput checks Go-side expectations on start --exec-script PTY output (harness markers + zoa run JSON).
func assertBoundaryExecScriptOutput(scriptOut string) (executionID string, exec map[string]interface{}) {
	Expect(scriptOut).To(ContainSubstring(boundaryExecOKMarker),
		"exec script did not finish — output:\n%s", scriptOut)

	jsonStr := extractBoundaryExecScriptJSON(scriptOut, boundaryExecJSONMarker)
	Expect(jsonStr).NotTo(BeEmpty(), "expected %s line in exec-script output:\n%s", boundaryExecJSONMarker, scriptOut)
	Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(),
		"expected zoa run -o json execution object in exec-script output:\n%s", scriptOut)
	Expect(exec["status"]).To(Equal("succeeded"))
	Expect(exec["action"]).To(Equal("get_resource"))
	Expect(exec["scope"]).To(Equal("kube"))

	out, ok := exec["output"].([]interface{})
	Expect(ok).To(BeTrue(), "execution output should be a node list array, got: %T", exec["output"])
	Expect(out).NotTo(BeEmpty(), "expected at least one node from get_resource inside boundary")

	executionID, _ = exec["id"].(string)
	Expect(executionID).NotTo(BeEmpty(), "execution id missing from exec-script zoa run JSON")
	return executionID, exec
}

// assertExecutionIdentityBridgeAttribution verifies boundary zoa run was attributed via the
// identity bridge (execution operator + session_id match the Access session row).
func assertExecutionIdentityBridgeAttribution(exec map[string]interface{}, row sessionRow) {
	op, _ := exec["operator"].(string)
	Expect(op).NotTo(BeEmpty(), "execution operator")
	Expect(op).To(Equal(row.Operator),
		"identity bridge: execution operator should match boundary session row")

	sid, _ := exec["session_id"].(string)
	Expect(sid).NotTo(BeEmpty(), "execution session_id (expected identity bridge hit from boundary task)")
	Expect(sid).To(Equal(row.SessionID),
		"identity bridge: execution session_id should match boundary session row")
}

func expectActiveSessionRow(deployment, targetID, rawID string) sessionRow {
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
	Expect(row).NotTo(BeNil(), "active session %q for target %q not in list: %v", rawID, targetID, rows)
	return *row
}

var _ = Describe("zoa session exec", Serial, func() {
	for _, tgt := range targets {
		tgt := tgt

		Describe(tgt.Name, Ordered, func() {
			var (
				deployment     string
				targetID       string
				compoundID     string
				rawID          string
				boundaryExecID string
				sessionReady   bool
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

			It("runs operator-path checks via session start --exec-script (automation path)", func() {
				script := boundaryExecScript(reasonValue, deployment, targetID)
				scriptOut, err := runZoaAccess(
					"session", "start", deployment, targetID,
					"--reason", reasonValue,
					"--exec-script", script,
					"--exec-script-timeout", "5m",
				)
				Expect(err).NotTo(HaveOccurred(), "start --exec-script output:\n%s", scriptOut)
				var parseErr error
				compoundID, parseErr = cli.ParseCompoundSessionIDFromCLIOutput(scriptOut)
				Expect(parseErr).NotTo(HaveOccurred(), scriptOut)
				_, rawID, parseErr = cli.ParseSessionID(compoundID)
				Expect(parseErr).NotTo(HaveOccurred())
				Expect(compoundID).NotTo(BeEmpty())
				Expect(rawID).NotTo(BeEmpty())

				var execFromScript map[string]interface{}
				boundaryExecID, execFromScript = assertBoundaryExecScriptOutput(scriptOut)
				row := expectActiveSessionRow(deployment, targetID, rawID)
				assertExecutionIdentityBridgeAttribution(execFromScript, row)
			})

			It("reconnects with a second ECS Exec join --exec-script on the same session", func() {
				Expect(compoundID).NotTo(BeEmpty(), "start --exec-script spec must run first")
				Expect(rawID).NotTo(BeEmpty(), "start --exec-script spec must run first")

				row := expectActiveSessionRow(deployment, targetID, rawID)
				Expect(row.Operator).NotTo(BeEmpty(), "session list must include operator for PS1 transcript checks")

				reconnectOut, err := runZoaAccess(
					"session", "join", compoundID,
					"--exec-script", boundaryExecReconnectScript,
					"--exec-script-timeout", "2m",
				)
				Expect(err).NotTo(HaveOccurred(), reconnectOut)
				assertBoundaryExecScriptTranscript(reconnectOut, deployment, targetID, reasonValue, compoundID, row.Operator)
				Expect(reconnectOut).To(ContainSubstring("Client:"), reconnectOut)
			})

			It("records exec_session_ids after exec-script join", func() {
				Expect(rawID).NotTo(BeEmpty(), "exec-script spec must run first")

				row := expectActiveSessionRow(deployment, targetID, rawID)
				Expect(row.ExecSessionIDs).NotTo(BeEmpty(),
					"expected at least one ECS Exec session id after exec-script join")
				Expect(row.ExecSessionIDs).To(HaveLen(2),
					"expected two exec joins (start --exec-script + reconnect join --exec-script); got %v", row.ExecSessionIDs)
			})

			It("persists boundary TA execution retrievable via zoa get", func() {
				Expect(boundaryExecID).NotTo(BeEmpty(), "first exec-script spec must capture execution id")
				Expect(rawID).NotTo(BeEmpty(), "first exec-script spec must capture session id")

				row := expectActiveSessionRow(deployment, targetID, rawID)

				out, err := runZoa(tgt, "get", boundaryExecID, "-o", "json", "--include-output")
				Expect(err).NotTo(HaveOccurred(), out)
				jsonStr := extractJSON(out)
				var exec map[string]interface{}
				Expect(json.Unmarshal([]byte(jsonStr), &exec)).To(Succeed(), out)
				Expect(exec["id"]).To(Equal(boundaryExecID))
				Expect(exec["action"]).To(Equal("get_resource"))
				Expect(exec["status"]).To(Equal("succeeded"))
				assertExecutionIdentityBridgeAttribution(exec, row)
			})

			It("terminates the boundary session after exec-script automation", func() {
				Expect(compoundID).NotTo(BeEmpty())
				termOut, err := runZoaAccess("session", "terminate", compoundID)
				Expect(err).NotTo(HaveOccurred(), termOut)
			})
		})
	}
})

// Exec-script failure path: join --exec-script with a script that exits non-zero.
var _ = Describe("zoa session exec script failures", func() {
	for _, tgt := range targets {
		tgt := tgt

		It("fails when join --exec-script script exits non-zero", func() {
			skipUnlessSessionE2E()
			ctx, err := resolveSessionE2EContext(tgt.TargetType)
			Expect(err).NotTo(HaveOccurred())
			dep := ctx.Deployment
			targetID := ctx.TargetID

			startOut, err := runZoaAccess(
				"session", "start", dep, targetID,
				"--reason", reasonValue,
				"--no-connect", "-o", "json",
			)
			Expect(err).NotTo(HaveOccurred(), startOut)
			started, err := parseSessionStartJSON(startOut)
			Expect(err).NotTo(HaveOccurred(), startOut)
			compoundID := started.SessionID

			_, err = runZoaAccess(
				"session", "join", compoundID,
				"--exec-script", "false",
				"--exec-script-timeout", "2m",
			)
			Expect(err).To(HaveOccurred(), "exec-script join should fail when script exits non-zero")

			termOut, err := runZoaAccess("session", "terminate", compoundID)
			Expect(err).NotTo(HaveOccurred(), termOut)
			Expect(termOut).NotTo(BeEmpty())
		})
	}
})
