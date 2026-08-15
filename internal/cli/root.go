// Package cli wires up the mison command tree.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the mison root command with all subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "mison",
		Short: "Reproduce your development environment anywhere",
		Long: `mison keeps development environments in sync across machines.

It uses mise as the installation engine and a GitHub repository as the
source of truth for your tool declarations (mise.toml).

Core workflow:
  mison init              set up this machine (mise, gh auth, env repo)
  mison install <tools>   add tools to the environment and install them
  mison uninstall <tools> remove tools from the environment
  mison sync              pull latest declaration and apply it
  mison status            compare declaration with installed tools`,
		Version: "0.0.1-dev",
	}

	root.AddCommand(
		newInitCmd(),
		newInstallCmd(),
		newUninstallCmd(),
		newSyncCmd(),
		newStatusCmd(),
	)
	return root
}

// Execute runs the root command.
func Execute() error {
	return NewRootCmd().Execute()
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Register this machine with a mison environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("init: not implemented yet")
		},
	}
}

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <tool>...",
		Short: "Add tools to the environment and install them",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("install: not implemented yet")
		},
	}
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <tool>...",
		Short: "Remove tools from the environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("uninstall: not implemented yet")
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull the latest environment and apply it",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("sync: not implemented yet")
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Compare the declaration with installed tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("status: not implemented yet")
		},
	}
}
