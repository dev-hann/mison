// Package gitrepo provides atomic git and gh commands over an injected
// service.Runner. It contains no sync policy — decisions live in
// usecase.
package gitrepo

import (
	"strings"

	"github.com/dev-hann/mison/internal/service"
)

// Repo runs atomic git commands inside a working directory.
type Repo struct {
	r   service.Runner
	dir string
}

// New builds a git Repo bound to dir.
func New(r service.Runner, dir string) *Repo { return &Repo{r: r, dir: dir} }

// Dir returns the working directory.
func (g *Repo) Dir() string { return g.dir }

func (g *Repo) run(args ...string) (string, error) {
	return g.r.Run(nil, "git", append([]string{"-C", g.dir}, args...)...)
}

// IsRepo reports whether dir is a git repository.
func (g *Repo) IsRepo() bool {
	_, err := g.run("rev-parse", "--git-dir")
	return err == nil
}

// Init turns dir into a fresh git repo with main as initial branch.
func (g *Repo) Init() error {
	_, err := g.run("init", "-b", "main")
	return err
}

// RemoteAdd registers origin.
func (g *Repo) RemoteAdd(url string) error {
	_, err := g.run("remote", "add", "origin", url)
	return err
}

// RemoteSetURL points origin at a different repository (re-binding).
func (g *Repo) RemoteSetURL(url string) error {
	_, err := g.run("remote", "set-url", "origin", url)
	return err
}

// RemoteURL returns the origin URL ("" when absent).
func (g *Repo) RemoteURL() string {
	out, err := g.run("remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Rev resolves a revision to a commit hash.
func (g *Repo) Rev(rev string) (string, error) {
	out, err := g.run("rev-parse", rev)
	return strings.TrimSpace(out), err
}

// ShowFile returns a file's content at a revision.
func (g *Repo) ShowFile(rev, path string) (string, error) {
	return g.run("show", rev+":"+path)
}

// IsClean reports whether the worktree has no changes.
func (g *Repo) IsClean() bool {
	out, err := g.run("status", "--porcelain")
	return err == nil && strings.TrimSpace(out) == ""
}

// StageAll stages every change.
func (g *Repo) StageAll() error {
	_, err := g.run("add", "-A")
	return err
}

// Commit records staged changes.
func (g *Repo) Commit(message string) error {
	_, err := g.run("commit", "-m", message)
	return err
}

// HasIdentity reports whether a git user.email is configured.
func (g *Repo) HasIdentity() bool {
	_, err := g.run("config", "user.email")
	return err == nil
}

// SetIdentityFallback configures a repo-local mison identity so mison
// never blocks on machines without global git config.
func (g *Repo) SetIdentityFallback() error {
	if _, err := g.run("config", "user.email", "mison@local"); err != nil {
		return err
	}
	_, err := g.run("config", "user.name", "mison")
	return err
}

// Fetch updates origin refs.
func (g *Repo) Fetch() error {
	_, err := g.run("fetch", "origin")
	return err
}

// Push moves HEAD to origin main.
func (g *Repo) Push() error {
	_, err := g.run("push", "origin", "HEAD:main")
	return err
}

// ResetHard resets the worktree and HEAD to rev.
func (g *Repo) ResetHard(rev string) error {
	_, err := g.run("reset", "--hard", rev)
	return err
}

// MergeBase returns the common ancestor of two revisions. An error
// (unrelated histories) yields "" plus the error.
func (g *Repo) MergeBase(a, b string) (string, error) {
	out, err := g.run("merge-base", a, b)
	return strings.TrimSpace(out), err
}
