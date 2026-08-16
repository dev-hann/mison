// Package cli wires the mison command tree: cobra parsing and adapter
// assembly only — all logic lives in usecase.
package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dev-hann/mison/internal/repo/gitrepo"
	"github.com/dev-hann/mison/internal/repo/miserepo"
	"github.com/dev-hann/mison/internal/service"
	"github.com/dev-hann/mison/internal/usecase"
)

// NewRootCmd builds the mison root command with all subcommands.
func NewRootCmd(f *usecase.Flows, version string) *cobra.Command {
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

	root.AddCommand(
		newInitCmd(f),
		newInstallCmd(f),
		newUninstallCmd(f),
		newSyncCmd(f),
		newStatusCmd(f),
	)
	return root
}

// Execute runs the root command with real dependencies.
func Execute(version string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	runner := service.New()
	f := &usecase.Flows{
		Home: home,
		Mise: miserepo.New(runner, home),
		Look: exec.LookPath,
		Git:  func(dir string) usecase.EnvRepoIface { return usecase.NewEngine(gitrepo.New(runner, dir)) },
		Gh:   gitrepo.NewGitHub(runner, home),
	}
	term := NewTermUI(os.Stdout, os.Stdin)
	f.UI = term
	f.Ask = term
	return NewRootCmd(f, version).Execute()
}

// addOSFlags registers --mac and --linux with optional arch values:
// --mac, --mac=arm64, --linux, --linux=x64.
func addOSFlags(fl *pflag.FlagSet) {
	fl.String("mac", "", "restrict install to macOS")
	fl.String("linux", "", "restrict install to Linux")
	fl.Lookup("mac").NoOptDefVal = "macos"
	fl.Lookup("linux").NoOptDefVal = "linux"
}

// osSpecFromFlags combines OS flags into a mise os spec ("" = none).
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

// addConflictFlags registers non-interactive conflict resolution.
func addConflictFlags(fl *pflag.FlagSet) {
	fl.Bool("ours", false, "keep this machine's version on conflict")
	fl.Bool("theirs", false, "accept the remote version on conflict")
}

func conflictPolicy(cmd *cobra.Command) (usecase.ConflictPolicy, error) {
	ours, _ := cmd.Flags().GetBool("ours")
	theirs, _ := cmd.Flags().GetBool("theirs")
	if ours && theirs {
		return usecase.PolicyAsk, fmt.Errorf("--ours and --theirs are mutually exclusive")
	}
	if ours {
		return usecase.PolicyOurs, nil
	}
	if theirs {
		return usecase.PolicyTheirs, nil
	}
	return usecase.PolicyAsk, nil
}

func boolFlag(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}
