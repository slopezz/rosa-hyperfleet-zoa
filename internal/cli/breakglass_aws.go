package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass/active"
)

// writeBreakglassAWSProfile writes ~/.config/zoa/breakglass/aws-credentials as [default]
// (single active aws grant; plain `aws` CLI, no AWS_PROFILE).
func writeBreakglassAWSProfile(home, requestID string, aws map[string]interface{}) error {
	_ = requestID
	if aws == nil {
		return nil
	}
	accessKey := strings.TrimSpace(fmt.Sprint(aws["access_key_id"]))
	if accessKey == "" {
		return nil
	}
	secretKey := fmt.Sprint(aws["secret_access_key"])
	sessionToken := fmt.Sprint(aws["session_token"])
	region := fmt.Sprint(aws["region"])
	block := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\naws_session_token = %s\n",
		accessKey, secretKey, sessionToken)
	if region != "" && region != "<nil>" {
		block += fmt.Sprintf("region = %s\n", region)
	}
	dir := active.ConfigDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := active.AWSCredentialsPath(home)
	return os.WriteFile(path, []byte(block), 0o600)
}
