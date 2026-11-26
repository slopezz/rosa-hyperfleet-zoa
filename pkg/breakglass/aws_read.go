package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name: "aws_read",
			Scope: ScopeAWS,
			Type:        TypeRead,
			Description: "AWS ReadOnlyAccess in the boundary target account (RC or MC).",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			Capabilities: []string{
				"Managed policy ReadOnlyAccess in the session account",
			},
			Restrictions: []string{
				"No write or elevated IAM actions (use aws_write / aws_elevated)",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-read",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
			Examples: []string{
				"zoa breakglass request aws_read --reason ROSAENG-1234",
				"zoa breakglass request aws_read --reason ROSAENG-1234 --wait",
				"zoa breakglass request aws_read --reason ROSAENG-1234 --wait --wait-timeout 10m --wait-poll-interval 1m",
			},
		},
	})
}
