package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newUpdateCmd(f *usecase.Flows) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [<tool>...]",
		Short: "Re-resolve fuzzy tool versions and install the updates",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return f.RunUpdate(args, dryRun, policy)
		},
	}
	cmd.Flags().Bool("dry-run", false, "show available updates without applying")
	addConflictFlags(cmd.Flags())
	return cmd
}
