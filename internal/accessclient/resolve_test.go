package accessclient

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAccessClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AccessClient Suite")
}

var _ = Describe("extractNameFromARN", func() {
	DescribeTable("When given an IAM/STS ARN it should return the identity segment",
		func(arn, expected string) {
			Expect(extractNameFromARN(arn)).To(Equal(expected))
		},
		Entry("assumed-role with session name",
			"arn:aws:sts::811685182089:assumed-role/rrp-admin/slopezma",
			"slopezma",
		),
		Entry("assumed-role with longer session name",
			"arn:aws:sts::811685182089:assumed-role/AWSReservedSSO_PowerUser/jane.doe@example.com",
			"jane.doe@example.com",
		),
		Entry("IAM user",
			"arn:aws:iam::123456789012:user/slopezma",
			"slopezma",
		),
		Entry("IAM user with path",
			"arn:aws:iam::123456789012:user/engineering/slopezma",
			"slopezma",
		),
		Entry("federated user",
			"arn:aws:sts::123456789012:federated-user/slopezma",
			"slopezma",
		),
		Entry("root account",
			"arn:aws:iam::123456789012:root",
			"",
		),
		Entry("empty string",
			"",
			"",
		),
		Entry("malformed ARN with no resource slash",
			"arn:aws:iam::123456789012:role-only",
			"",
		),
	)
})
