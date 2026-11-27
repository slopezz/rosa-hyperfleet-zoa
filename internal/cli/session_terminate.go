package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSessionTerminateCommand(opts *GlobalOptions) *cobra.Command {
	run := func(cmd *cobra.Command, args []string) error {
		deployment, rawID, err := ParseSessionID(args[0])
		if err != nil {
			return err
		}

		if opts.APIURL == "" {
			opts.Deployment = deployment
		}

		c, err := getClient(opts)
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		if err := c.SessionTerminate(cmd.Context(), rawID); err != nil {
			return fmt.Errorf("terminating session: %w", err)
		}

		fmt.Printf("Session %s terminated\n", args[0])
		return nil
	}

	return &cobra.Command{
		Use:   "terminate <deployment/session-id>",
		Short: "Terminate a boundary session",
		Long: `Terminate a boundary session by its compound ID (deployment/session-id).

Stops the ECS task and marks the session terminated in DynamoDB. This is final —
the task cannot be restarted. The compound ID is returned by 'zoa session start'
and 'zoa session list'.`,
		Example: `  zoa session terminate us-east-1/1989937d-0889-485d-b64e-facee9e0150d`,
		Args:    cobra.ExactArgs(1),
		RunE:    run,
	}
}