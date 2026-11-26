package breakglass

// AWS break-glass profile ids (executor maps name → BREAKGLASS_AWS_*_ROLE_ARN, like TA scope+type).
const (
	ProfileAWSRead     = "aws_read"
	ProfileAWSWrite    = "aws_write"
	ProfileAWSElevated = "aws_elevated"
)

// KnownAWSProfiles is the set of registered aws-plane profile names.
var KnownAWSProfiles = map[string]bool{
	ProfileAWSRead:     true,
	ProfileAWSWrite:    true,
	ProfileAWSElevated: true,
}