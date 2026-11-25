package breakglass

import "github.com/openshift-online/rosa-seaplane-zoa/pkg/actions"

func init() {
	register(&Definition{
		Metadata: ProfileMetadata{
			Name: "aws-elevated",
			Scope: ScopeAWS,
			Type:        TypeElevated,
			Description: "PowerUserAccess with Deny guards on ZOA control-plane immutability.",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultRequestHours: 8,
			},
			AwsRoleKey: AwsRoleKeyElevated,
			Capabilities: []string{
				"PowerUserAccess in the session account (e.g. EC2 terminate for stuck nodes on MC)",
				"Immutability Deny: ZOA S3 artifacts, Dynamo rows/tables, ZOA Lambdas, boundary ECS Exec",
			},
			Restrictions: []string{
				"IAM/org mutation and broad secrets/KMS paths remain denied (Terraform policy sidebar)",
			},
			IAMComposition: []string{
				"Role: {cluster_id}-zoa-bg-aws-elevated",
			},
			TargetTypes: []string{actions.TargetTypeRC, actions.TargetTypeMC},
		},
	})
}
