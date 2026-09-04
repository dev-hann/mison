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
			f.NoShellSetup, _ = cmd.Flags().GetBool("no-shell-setup")
			f.Account, _ = cmd.Flags().GetString("account")
			return f.RunInit(repoName)
		},
	}
	cmd.Flags().String("repo", usecase.DefaultRepoName, "environment repository name (owner/name or name)")
	cmd.Flags().Bool("no-shell-setup", false, "do not modify shell rc files (mise activation)")
	cmd.Flags().String("account", "", "pin the GitHub account for this machine (stored; enforced on push/pull)")
	return cmd
}
