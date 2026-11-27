package cli

import (
	"github.com/spf13/cobra"
)

// helpSessionExecScriptAutomation is shared by session start/join Long help for consistent --exec-script wording.
const helpSessionExecScriptAutomation = `Scripted boundary session (--exec-script): run a bash script inside the task via ECS Exec (session-manager-plugin with a PTY). For runbooks, PagerDuty, CI, and e2e — same Access join, scoped credentials, and CloudWatch audit as interactive join. Incompatible with -o json. With 'session start', also incompatible with --no-connect; use 'session start --no-connect -o json', capture session_id (e.g. jq -r .session_id), then 'session join --exec-script'.`

const helpSessionOverview = `Boundary sessions are audited ECS Fargate shells in a target VPC (RC or MC). You work from the laptop via the Access Lambda (SigV4 invoker role), not deployment-admin kubeconfig.

Typical flow:
  zoa deployments  — list deployment names
  zoa targets <deployment>  — list target ids for that deployment
  zoa session start <deployment> <target> --reason <ticket>  — provisions task; default CLI then joins ECS Exec
  exit  — disconnects Exec only; boundary task stays until terminate or reaper
  zoa session join <deployment>/<session-id>  — reconnect (requires session-manager-plugin on PATH)
  zoa session terminate <deployment>/<session-id>  — stop task and mark session terminated
  zoa session list / history  — your sessions vs fleet audit (see each command --help)

Sessions are bound to your invoker identity: only you can join or terminate your session. A reason (Jira or PagerDuty) is required on start.

Discovery: deployment metadata from Central SSM (try us-east-1 then eu-west-1, or --deployments-region). Optional --access-url and --invoker-role-arn skip deployment SSM (both required). Compound id: <deployment>/<session-id>. Subcommand details, flags, and examples: zoa session <command> --help.`

func newSessionCommand(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage boundary sessions",
		Long:  helpSessionOverview,
		Example: `  zoa session start us-east-1 regional --reason ROSAENG-1234
  zoa session list us-east-1
  zoa session join us-east-1/1989937d-0889-485d-b64e-facee9e0150d
  zoa session terminate us-east-1/1989937d-0889-485d-b64e-facee9e0150d
  zoa session history us-east-1 --since 7d`,
	}

	cmd.AddCommand(
		newSessionStartCommand(opts),
		newSessionTerminateCommand(opts),
		newSessionJoinCommand(opts),
		newSessionListCommand(opts),
		newSessionHistoryCommand(opts),
	)

	return cmd
}