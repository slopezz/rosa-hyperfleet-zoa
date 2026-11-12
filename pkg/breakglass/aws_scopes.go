package breakglass

const (
	AwsRoleKeyRead     = "aws-read"
	AwsRoleKeyWrite    = "aws-write"
	AwsRoleKeyElevated = "aws-elevated"
)

func init() {
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "aws-read",
			Plane:       PlaneAWS,
			Type:        TypeRead,
			Description: "Regional AWS read (Describe/List/Get) via scoped STS role",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
			AwsRoleKey:  AwsRoleKeyRead,
			TargetTypes: []string{},
		},
	})
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "aws-write",
			Plane:       PlaneAWS,
			Type:        TypeWrite,
			Description: "Constrained regional AWS mutations via scoped STS role",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   4,
				DefaultPendingHours: 24,
			},
			AwsRoleKey: AwsRoleKeyWrite,
		},
	})
	register(&Definition{
		Metadata: ScopeMetadata{
			ID:          "aws-elevated",
			Plane:       PlaneAWS,
			Type:        TypeElevated,
			Description: "Near-admin AWS access with short deny sidebar",
			Authorization: Authorization{
				Approval:            "none",
				DefaultGrantHours:   1,
				DefaultPendingHours: 8,
			},
			AwsRoleKey: AwsRoleKeyElevated,
		},
	})
}
