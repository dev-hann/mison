package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newUninstallCmd(f *usecase.Flows) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall <tool>...",
		Short: "Remove tools from the environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			return f.RunUninstall(args, boolFlag(cmd, "yes"), policy)
		},
	}
	cmd.Flags().BoolP("yes", "y", false, "skip confirmation")
	addConflictFlags(cmd.Flags())
	return cmd
}
