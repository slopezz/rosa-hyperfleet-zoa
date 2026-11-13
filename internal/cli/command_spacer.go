package cli

import (
	"fmt"
	"os"
)

// CommandSpacer prints a blank line on stderr so the shell prompt does not hug the last output line.
func CommandSpacer() {
	fmt.Fprintln(os.Stderr)
}
