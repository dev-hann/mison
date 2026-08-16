package cli

import (
	"fmt"
	"strings"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/gitclient"
)

// Repo is the git surface cli depends on (satisfied by *gitclient.Git).
type Repo interface {
	IsRepo() bool
	Init() error
	Connect(url string) error
	RemoteAdd(url string) error
	RemoteURL() string
	RemoteIsEmpty() bool
	SyncStatus() (gitclient.SyncInfo, error)
	SmartPush(message string, resolve gitclient.Resolver) ([]string, error)
	SmartPull(resolve gitclient.Resolver) ([]string, error)
}

// GhClient is the gh surface cli depends on (satisfied by *gh.Client).
type GhClient interface {
	IsInstalled() bool
	AuthStatus() bool
	AuthLogin() error
	SetupGit() error
	RepoExists(name string) bool
	RepoURL(name string) (string, error)
	CreatePrivateRepo(name string) (string, error)
}

// ConflictPolicy decides how same-tool conflicts resolve non-interactively.
type ConflictPolicy int

// Conflict resolution policies.
const (
	PolicyAsk ConflictPolicy = iota
	PolicyOurs
	PolicyTheirs
)

// makeResolver builds a gitclient.Resolver from a policy and the UI.
func (a *App) makeResolver(policy ConflictPolicy) gitclient.Resolver {
	return func(conflicts []env.Conflict) ([]env.Tool, error) {
		out := make([]env.Tool, 0, len(conflicts))
		for _, c := range conflicts {
			switch policy {
			case PolicyOurs:
				out = append(out, pickSide(c.Local, c.Remote))
			case PolicyTheirs:
				out = append(out, pickSide(c.Remote, c.Local))
			default:
				tool, err := a.Ask.ResolveConflict(c)
				if err != nil {
					return nil, err
				}
				out = append(out, tool)
			}
		}
		return out, nil
	}
}

// pickSide prefers the primary side, falling back to the other when the
// primary is a removal (empty tool).
func pickSide(primary, fallback env.Tool) env.Tool {
	if primary.Name == "" {
		return fallback
	}
	return primary
}

// commitAndPush applies the DESIGN.md push policy after a declaration
// change: no repo → skip silently; divergence → reconcile; offline →
// warn and defer to the next sync.
func (a *App) commitAndPush(message string, policy ConflictPolicy) {
	repo := a.Git(a.layout().EnvDir)
	if !repo.IsRepo() {
		return
	}
	merged, err := repo.SmartPush(message, a.makeResolver(policy))
	if err != nil {
		a.UI.Warn("could not push — will retry on next sync (" + err.Error() + ")")
		return
	}
	if len(merged) > 0 {
		a.UI.Synced(fmt.Sprintf("Remote had new changes (%s) — merged automatically", strings.Join(merged, ", ")))
	}
}
