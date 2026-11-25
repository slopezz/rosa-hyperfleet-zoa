package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name: "aws-write",
			Scope: ScopeAWS,
			Type:        TypeWrite,
			Description: "ReadOnlyAccess plus RC-only reboot/start/stop. On MC, same effective IAM as aws-read.",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			AwsRoleKey: AwsRoleKeyWrite,
			Capabilities: []string{
				"Includes ReadOnlyAccess (aws-read)",
				"RC only: EC2 start/stop/reboot; RDS RebootDBCluster; ElastiCache RebootCacheCluster",
			},
			Restrictions: []string{
				"MC target: same effective IAM as aws-read (no state-change mutations)",
				"No elevated actions (EC2 terminate, PowerUser); use aws-elevated",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-write",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
	})
}
