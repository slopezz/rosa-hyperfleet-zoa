package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/breakglass/active"
)

// writeBreakglassAWSProfile updates [zoa-bg-<requestId>] in ~/.config/zoa/breakglass/aws-credentials.
func writeBreakglassAWSProfile(home, requestID string, aws map[string]interface{}) error {
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
	profile := "zoa-bg-" + requestID
	block := fmt.Sprintf("[%s]\naws_access_key_id = %s\naws_secret_access_key = %s\naws_session_token = %s\n",
		profile, accessKey, secretKey, sessionToken)
	if region != "" && region != "<nil>" {
		block += fmt.Sprintf("region = %s\n", region)
	}
	dir := active.ConfigDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := active.AWSCredentialsPath(home)
	existing, _ := os.ReadFile(path)
	updated := mergeAWSCredentialsProfile(string(existing), profile, block)
	return os.WriteFile(path, []byte(updated), 0o600)
}

func mergeAWSCredentialsProfile(existing, profile, block string) string {
	marker := "[" + profile + "]"
	if existing == "" {
		return block
	}
	parts := strings.Split(existing, "\n")
	var out []string
	skip := false
	for _, line := range parts {
		if strings.TrimSpace(line) == marker {
			skip = true
			continue
		}
		if skip && strings.HasPrefix(strings.TrimSpace(line), "[") && line != marker {
			skip = false
		}
		if skip {
			continue
		}
		if strings.TrimSpace(line) != "" || len(out) > 0 {
			out = append(out, line)
		}
	}
	body := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if body != "" {
		body += "\n\n"
	}
	body += strings.TrimRight(block, "\n") + "\n"
	return body
}
