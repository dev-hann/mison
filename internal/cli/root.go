package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dev-hann/mison/internal/gh"
	"github.com/dev-hann/mison/internal/gitclient"
	"github.com/dev-hann/mison/internal/mise"
)

// NewRootCmd builds the mison root command with all subcommands.
func NewRootCmd(app *App, version string) *cobra.Command {
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
		Version: version,
	}

	install := newInstallCmd(app)
	addOSFlags(install.Flags())
	addConflictFlags(install.Flags())

	uninstall := newUninstallCmd(app)
	uninstall.Flags().BoolP("yes", "y", false, "skip confirmation")
	addConflictFlags(uninstall.Flags())

	sync := newSyncCmd(app)
	sync.Flags().Bool("prune", false, "remove undeclared tools without prompting")
	addConflictFlags(sync.Flags())

	initCmd := newInitCmd(app)
	initCmd.Flags().String("repo", gh.DefaultRepoName, "environment repository name (owner/name or name)")

	root.AddCommand(
		initCmd,
		install,
		uninstall,
		sync,
		newStatusCmd(app),
	)
	return root
}

// addConflictFlags registers non-interactive conflict resolution.
func addConflictFlags(f *pflag.FlagSet) {
	f.Bool("ours", false, "keep this machine's version on conflict")
	f.Bool("theirs", false, "accept the remote version on conflict")
}

func conflictPolicy(cmd *cobra.Command) (ConflictPolicy, error) {
	ours, _ := cmd.Flags().GetBool("ours")
	theirs, _ := cmd.Flags().GetBool("theirs")
	if ours && theirs {
		return PolicyAsk, fmt.Errorf("--ours and --theirs are mutually exclusive")
	}
	if ours {
		return PolicyOurs, nil
	}
	if theirs {
		return PolicyTheirs, nil
	}
	return PolicyAsk, nil
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
func Execute(version string) error {
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
		Git:      func(dir string) Repo { return gitclient.New(dir) },
		Gh:       gh.New(),
	}
	term := NewTermUI(app)
	app.UI = term
	app.Ask = term
	return NewRootCmd(app, version).Execute()
}

func newInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Register this machine with a mison environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoName, _ := cmd.Flags().GetString("repo")
			return app.RunInit(repoName)
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
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			return app.RunInstall(args, osFlag, policy)
		},
	}
}

func newUninstallCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <tool>...",
		Short: "Remove tools from the environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			return app.RunUninstall(args, boolFlag(cmd, "yes"), policy)
		},
	}
}

func newSyncCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull the latest environment and apply it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			policy, err := conflictPolicy(cmd)
			if err != nil {
				return err
			}
			return app.RunSync(boolFlag(cmd, "prune"), policy)
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
