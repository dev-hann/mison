package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newInitCmd(f *usecase.Flows) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Register this machine with a mison environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoName, _ := cmd.Flags().GetString("repo")
			return f.RunInit(repoName)
		},
	}
	cmd.Flags().String("repo", usecase.DefaultRepoName, "environment repository name (owner/name or name)")
	return cmd
}
