package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "aws-read",
			Plane:       PlaneAWS,
			Type:        TypeRead,
			Description: "AWS ReadOnlyAccess in the boundary target account (RC or MC).",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultRequestHours: 24,
			},
			AwsRoleKey: AwsRoleKeyRead,
			Capabilities: []string{
				"Managed policy ReadOnlyAccess in the session account",
			},
			Restrictions: []string{
				"No write or elevated IAM actions (use aws-write / aws-elevated)",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-read",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
	})
}
