package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dev-hann/mison/internal/mise"
)

// NewRootCmd builds the mison root command with all subcommands.
func NewRootCmd(app *App) *cobra.Command {
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
		Version: "0.1.0-dev",
	}

	install := newInstallCmd(app)
	addOSFlags(install.Flags())

	uninstall := newUninstallCmd(app)
	uninstall.Flags().BoolP("yes", "y", false, "skip confirmation")

	sync := newSyncCmd(app)
	sync.Flags().Bool("prune", false, "remove undeclared tools without prompting")

	root.AddCommand(
		newInitCmd(app),
		install,
		uninstall,
		sync,
		newStatusCmd(app),
	)
	return root
}

// addOSFlags registers --mac and --linux with optional arch values:
// --mac, --mac=arm64, --linux, --linux=x64.
func addOSFlags(f *pflag.FlagSet) {
	f.String("mac", "", "restrict install to macOS")
	f.String("linux", "", "restrict install to Linux")
	f.Lookup("mac").NoOptDefVal = "macos"
	f.Lookup("linux").NoOptDefVal = "linux"
}

// osSpecFromFlags combines OS flags into a mise os spec ("" = no restriction).
func osSpecFromFlags(cmd *cobra.Command) (string, error) {
	mac, _ := cmd.Flags().GetString("mac")
	linux, _ := cmd.Flags().GetString("linux")
	if mac != "" && linux != "" {
		return "", fmt.Errorf("--mac and --linux are mutually exclusive")
	}
	for _, pair := range []struct{ flag, val string }{
		{"mac", mac}, {"linux", linux},
	} {
		if pair.val == "" {
			continue
		}
		if pair.val == "macos" || pair.val == "linux" {
			return pair.val, nil // bare flag, NoOptDefVal
		}
		return pair.flag + "/" + pair.val, nil // --mac=arm64 → macos/arm64
	}
	return "", nil
}

// Execute runs the root command with real dependencies.
func Execute() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	app := &App{
		Home:     home,
		Stdout:   os.Stdout,
		In:       os.Stdin,
		Mise:     mise.NewManager(mise.OsExecutor{}, home),
		LookPath: exec.LookPath,
	}
	return NewRootCmd(app).Execute()
}

func newInitCmd(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Register this machine with a mison environment",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("init: not implemented yet (M2)")
		},
	}
}

func newInstallCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "install <tool>[@<version>]...",
		Short: "Add tools to the environment and install them",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			osFlag, err := osSpecFromFlags(cmd)
			if err != nil {
				return err
			}
			return app.RunInstall(args, osFlag)
		},
	}
}

func newUninstallCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <tool>...",
		Short: "Remove tools from the environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunUninstall(args, boolFlag(cmd, "yes"))
		},
	}
}

func newSyncCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull the latest environment and apply it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.RunSync(boolFlag(cmd, "prune"))
		},
	}
}

func newStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Compare the declaration with installed tools",
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.RunStatus()
		},
	}
}

func boolFlag(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}
