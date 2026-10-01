package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

func printSessionDispatched(w io.Writer, compoundID, target string) {
	fmt.Fprintf(w, "✓ %s [%s]\n", compoundID, target)
}

func printSessionJoinHint(w io.Writer, compoundID string) {
	fmt.Fprintf(w, "  use 'zoa session join %s' to connect\n", compoundID)
}

func accessClientForJoin(c APIClient) APIClient {
	if jc, ok := c.(*client.Client); ok {
		return jc.WithTimeout(5 * time.Minute)
	}
	return c
}

func sessionStartTarget(deployment, flagTarget string, resp *client.SessionStartResponse) string {
	if resp.Target != "" {
		return resp.Target
	}
	return flagTarget
}

func sessionProgressWriter() io.Writer {
	return os.Stderr
}
