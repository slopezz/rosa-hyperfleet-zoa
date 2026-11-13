package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:    "aws-elevated",
			Plane: PlaneAWS,
			Type:  TypeElevated,
			Description: "PowerUserAccess with Deny guards on ZOA control-plane immutability.",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultPendingHours: 8,
			},
			AwsRoleKey: AwsRoleKeyElevated,
			Capabilities: []string{
				"PowerUserAccess in the session account",
				"Immutability Deny: ZOA S3 artifacts, Dynamo rows/tables, ZOA Lambdas, boundary ECS Exec",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-elevated",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
	})
}
