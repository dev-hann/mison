package cli

import (
	"fmt"
	"strings"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/gitclient"
	"github.com/dev-hann/mison/internal/ui"
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
				tool, err := a.promptConflict(c)
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

func (a *App) promptConflict(c env.Conflict) (env.Tool, error) {
	r := ui.New(a.Stdout)
	localDesc := "removed"
	if c.Local.Name != "" {
		localDesc = c.Local.Version
	}
	remoteDesc := "removed"
	if c.Remote.Name != "" {
		remoteDesc = c.Remote.Version
	}
	r.Fail(fmt.Sprintf("Conflict on %s (this machine: %s, remote: %s)", c.Name, localDesc, remoteDesc))
	r.Line("  [1] keep this machine  [2] accept remote")
	_, _ = fmt.Fprint(a.Stdout, "Choose [1/2]: ")

	choice := a.readLine()
	switch strings.TrimSpace(choice) {
	case "1":
		return pickSide(c.Local, c.Remote), nil
	case "2":
		return pickSide(c.Remote, c.Local), nil
	default:
		return env.Tool{}, fmt.Errorf("conflict on %s unresolved — aborting (local changes kept unpushed)", c.Name)
	}
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
		a.ui().Warn("could not push — will retry on next sync (" + err.Error() + ")")
		return
	}
	if len(merged) > 0 {
		a.ui().Synced(fmt.Sprintf("Remote had new changes (%s) — merged automatically", strings.Join(merged, ", ")))
	}
}
