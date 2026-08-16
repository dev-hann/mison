// Package gh wraps the GitHub CLI for mison's bootstrap chain
// (DESIGN.md: mise installs gh → device-flow login → setup-git).
// Thin exec wrappers; verified by e2e tests, not unit tests.
package gh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DefaultRepoName is the environment repository mison creates.
const DefaultRepoName = "mison-environment"

// Client drives the gh CLI.
type Client struct{}

// New builds a gh client.
func New() *Client { return &Client{} }

func (c *Client) run(interactive bool, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Env = miseEnv()
	out, err := func() ([]byte, error) {
		if interactive {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return nil, cmd.Run()
		}
		return cmd.CombinedOutput()
	}()
	return string(out), err
}

// miseEnv prepends mise shims so `gh` resolves after `mise install gh`.
func miseEnv() []string {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_DATA_HOME")
	base := home + "/.local/share"
	if xdg != "" {
		base = xdg
	}
	path := base + "/mise/shims:" + home + "/.local/bin:" + os.Getenv("PATH")
	env := []string{"PATH=" + path}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	return env
}

// IsInstalled reports whether gh responds to --version.
func (c *Client) IsInstalled() bool {
	_, err := c.run(false, "--version")
	return err == nil
}

// AuthStatus reports whether gh holds a valid GitHub token.
func (c *Client) AuthStatus() bool {
	_, err := c.run(false, "auth", "status")
	return err == nil
}

// AuthLogin runs the interactive device-flow login.
func (c *Client) AuthLogin() error {
	if _, err := c.run(true, "auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--web"); err != nil {
		return fmt.Errorf("gh auth login: %w", err)
	}
	return nil
}

// SetupGit configures git to use gh as credential helper.
func (c *Client) SetupGit() error {
	if _, err := c.run(false, "auth", "setup-git"); err != nil {
		return fmt.Errorf("gh auth setup-git: %w", err)
	}
	return nil
}

// RepoExists reports whether owner/name is visible.
func (c *Client) RepoExists(name string) bool {
	_, err := c.run(false, "repo", "view", name)
	return err == nil
}

// RepoURL returns the HTTPS clone URL for an existing repository.
func (c *Client) RepoURL(name string) (string, error) {
	out, err := c.run(false, "repo", "view", name, "--json", "url", "--jq", ".url")
	if err != nil {
		return "", fmt.Errorf("gh repo view %s: %w", name, err)
	}
	url := strings.TrimRight(strings.TrimSpace(out), "/")
	return url + ".git", nil
}

// CreatePrivateRepo creates a private repository and returns its HTTPS URL.
func (c *Client) CreatePrivateRepo(name string) (string, error) {
	if _, err := c.run(false, "repo", "create", name, "--private"); err != nil {
		return "", fmt.Errorf("gh repo create %s: %w", name, err)
	}
	return c.RepoURL(name)
}
