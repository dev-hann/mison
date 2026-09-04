package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newInstallCmd(f *usecase.Flows) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <tool>[@<version>]...",
		Short: "Add tools to the environment and install them",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			return f.RunInstall(args, policy)
		},
	}
	addConflictFlags(cmd.Flags())
	return cmd
}
