package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "aws-write",
			Plane:       PlaneAWS,
			Type:        TypeWrite,
			Description: "ReadOnlyAccess plus RC-only reboot/start/stop. On MC, same effective IAM as aws-read.",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
			AwsRoleKey: AwsRoleKeyWrite,
			Capabilities: []string{
				"RC: EC2 start/stop/reboot; RDS RebootDBCluster; ElastiCache RebootCacheCluster",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-write",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
	})
}
