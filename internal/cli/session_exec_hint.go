package cli

import (
	"fmt"
	"io"
	"time"
)

func printSessionExecEndedHint(w io.Writer, deployment, compoundID, deadlineRFC3339 string) {
	if compoundID == "" {
		return
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "==> ZOA boundary — Exec ended (ECS task may still be running)")
	if deadlineRFC3339 != "" {
		if t, err := time.Parse(time.RFC3339Nano, deadlineRFC3339); err == nil {
			fmt.Fprintf(w, "    Hard stop (UTC):        %s\n", t.UTC().Format("2006-01-02 15:04 UTC"))
		} else if t, err := time.Parse(time.RFC3339, deadlineRFC3339); err == nil {
			fmt.Fprintf(w, "    Hard stop (UTC):        %s\n", t.UTC().Format("2006-01-02 15:04 UTC"))
		}
	}
	fmt.Fprintf(w, "    Stop session:             zoa session stop %s\n", compoundID)
	fmt.Fprintf(w, "    Re-join session:          zoa session join %s\n", compoundID)
	fmt.Fprintf(w, "    Your sessions:            zoa session list %s\n", deployment)
	fmt.Fprintf(w, "    All operators (audit):    zoa session history %s\n", deployment)
	fmt.Fprintln(w, "    exit / SSM idle disconnect only closes Exec — stop the task from your laptop when done.")
	fmt.Fprintln(w, "")
}
