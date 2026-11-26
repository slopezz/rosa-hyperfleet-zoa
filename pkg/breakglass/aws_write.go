package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name: "aws_write",
			Scope: ScopeAWS,
			Type:        TypeWrite,
			Description: "ReadOnlyAccess plus RC-only reboot/start/stop. On MC, same effective IAM as aws_read.",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			Capabilities: []string{
				"Includes ReadOnlyAccess (aws_read)",
				"RC only: EC2 start/stop/reboot; RDS RebootDBCluster; ElastiCache RebootCacheCluster",
			},
			Restrictions: []string{
				"MC target: same effective IAM as aws_read (no state-change mutations)",
				"No elevated actions (EC2 terminate, PowerUser); use aws_elevated",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-write",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
			Examples: []string{
				"zoa breakglass request aws_write --reason ROSAENG-1234",
				"zoa breakglass request aws_write --reason ROSAENG-1234 --wait",
				"zoa breakglass request aws_write --reason ROSAENG-1234 --wait --wait-timeout 10m --wait-poll-interval 1m",
			},
		},
	})
}
