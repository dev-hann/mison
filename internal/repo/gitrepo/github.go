package gitrepo

import (
	"fmt"
	"strings"

	"github.com/dev-hann/mison/internal/service"
)

// GitHub runs atomic gh CLI commands. gh is resolved through mise
// shims so it works right after `mise install gh`.
type GitHub struct {
	r    service.Runner
	home string
}

// NewGitHub builds a gh client bound to the user's home directory.
func NewGitHub(r service.Runner, home string) *GitHub {
	return &GitHub{r: r, home: home}
}

func (c *GitHub) run(args ...string) (string, error) {
	return c.r.Run(service.MiseEnv(c.home), "gh", args...)
}

// IsInstalled reports whether gh responds to --version.
func (c *GitHub) IsInstalled() bool {
	_, err := c.run("--version")
	return err == nil
}

// AuthStatus reports whether gh holds a valid GitHub token.
func (c *GitHub) AuthStatus() bool {
	_, err := c.run("auth", "status")
	return err == nil
}

// Whoami returns the login of the ACTIVE gh account — with multiple
// accounts logged in, gh operates as whichever is active.
func (c *GitHub) Whoami() (string, error) {
	out, err := c.run("api", "user", "--jq", ".login")
	if err != nil {
		return "", fmt.Errorf("gh api user: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// AuthLogin runs the interactive device-flow login. The child process
// owns the terminal (stdio passthrough) — the documented exception to
// the interaction ports.
func (c *GitHub) AuthLogin() error {
	args := []string{"auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--web"}
	if err := c.r.RunTTY(service.MiseEnv(c.home), "gh", args...); err != nil {
		return fmt.Errorf("gh auth login: %w", err)
	}
	return nil
}

// SetupGit configures git to use gh as credential helper.
func (c *GitHub) SetupGit() error {
	if _, err := c.run("auth", "setup-git"); err != nil {
		return fmt.Errorf("gh auth setup-git: %w", err)
	}
	return nil
}

// RepoExists reports whether owner/name is visible.
func (c *GitHub) RepoExists(name string) bool {
	_, err := c.run("repo", "view", name)
	return err == nil
}

// RepoURL returns the HTTPS clone URL for an existing repository.
func (c *GitHub) RepoURL(name string) (string, error) {
	out, err := c.run("repo", "view", name, "--json", "url", "--jq", ".url")
	if err != nil {
		return "", fmt.Errorf("gh repo view %s: %w", name, err)
	}
	url := strings.TrimRight(strings.TrimSpace(out), "/")
	return url + ".git", nil
}

// LatestReleaseTag queries the GitHub API for a repo's latest release
// tag. Deliberately plain HTTP (curl), not gh — upgrading mison must
// work even when gh auth is broken.
func (c *GitHub) LatestReleaseTag(repo string) (string, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	out, err := c.r.Run(service.MiseEnv(c.home), "sh", "-c", "curl -fsSL "+url)
	if err != nil {
		return "", fmt.Errorf("latest release of %s: %w — %s", repo, err, strings.TrimSpace(out))
	}
	key := `"tag_name"`
	i := strings.Index(out, key)
	if i < 0 {
		return "", fmt.Errorf("latest release of %s: no tag_name in response", repo)
	}
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out[i+len(key):]), ":"))
	if !strings.HasPrefix(rest, `"`) {
		return "", fmt.Errorf("latest release of %s: malformed tag_name", repo)
	}
	vEnd := strings.Index(rest[1:], `"`)
	if vEnd < 0 {
		return "", fmt.Errorf("latest release of %s: malformed tag_name", repo)
	}
	return rest[1 : 1+vEnd], nil
}

// RunMisonInstaller runs the official install.sh (checksum-verified,
// keeps the mison.old backup) — the same path the README documents.
func (c *GitHub) RunMisonInstaller() error {
	script := "curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh"
	out, err := c.r.Run(service.MiseEnv(c.home), "sh", "-c", script)
	if err != nil {
		return fmt.Errorf("install mison: %w — %s", err, strings.TrimSpace(out))
	}
	return nil
}

// CreatePrivateRepo creates a private repository and returns its URL.
func (c *GitHub) CreatePrivateRepo(name string) (string, error) {
	if _, err := c.run("repo", "create", name, "--private"); err != nil {
		return "", fmt.Errorf("gh repo create %s: %w", name, err)
	}
	return c.RepoURL(name)
}
