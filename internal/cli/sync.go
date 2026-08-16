package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newSyncCmd(f *usecase.Flows) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull the latest environment and apply it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			return f.RunSync(boolFlag(cmd, "prune"), policy)
		},
	}
	cmd.Flags().Bool("prune", false, "remove undeclared tools without prompting")
	addConflictFlags(cmd.Flags())
	return cmd
}
