// Package gitclient wraps git operations for the environment repository.
// SmartPush/SmartPull implement the DESIGN.md §3 policy: fetch first,
// semantic 3-way merge on divergence, never a conflicted git state.
package gitclient

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dev-hann/mison/internal/env"
)

// Resolver settles same-tool conflicts produced by the semantic merge.
// Return the winning tools (empty Tool = removal wins on that side).
type Resolver func(conflicts []env.Conflict) ([]env.Tool, error)

// Git runs git commands inside a working directory.
type Git struct {
	dir string
}

// New builds a Git client bound to dir.
func New(dir string) *Git { return &Git{dir: dir} }

// Dir returns the working directory.
func (g *Git) Dir() string { return g.dir }

func (g *Git) run(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// IsRepo reports whether dir is a git repository.
func (g *Git) IsRepo() bool {
	_, err := g.run("rev-parse", "--git-dir")
	return err == nil
}

// Init turns dir into a fresh git repo with main as initial branch.
func (g *Git) Init() error {
	if _, err := g.run("init", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	return nil
}

// SyncState describes the local clone's relation to origin/main.
type SyncState int

// Sync states.
const (
	SyncUpToDate SyncState = iota
	SyncAhead
	SyncBehind
	SyncDiverged
)

// SyncInfo is the result of a read-only remote comparison.
type SyncInfo struct {
	State       SyncState
	RemoteAdded []string // tool names the remote gained (Behind)
	LocalAdded  []string // tool names only local has (Ahead/Diverged)
}

// SyncStatus fetches and compares the local declaration with the remote
// without modifying anything. Read-only counterpart to SmartPull.
func (g *Git) SyncStatus() (SyncInfo, error) {
	var info SyncInfo
	if err := g.fetch(); err != nil {
		return info, err
	}
	head, err := g.rev("HEAD")
	if err != nil {
		return info, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	remote, err := g.rev("origin/main")
	if err != nil {
		return info, fmt.Errorf("no remote main: %w", err)
	}

	if head == remote {
		info.State = SyncUpToDate
		return info, nil
	}

	mbOut, mbErr := g.run("merge-base", head, remote)
	mb := strings.TrimSpace(mbOut)
	if mbErr != nil {
		mb = "" // unrelated histories
	}

	localCfg, err := g.configAt(head)
	if err != nil {
		return info, err
	}
	remoteCfg, err := g.configAt(remote)
	if err != nil {
		return info, err
	}
	localTools, remoteTools := localCfg.Tools(), remoteCfg.Tools()

	switch mb {
	case remote:
		info.State = SyncAhead
		info.LocalAdded = diffNames(remoteTools, localTools)
	case head:
		info.State = SyncBehind
		info.RemoteAdded = diffNames(localTools, remoteTools)
	default:
		info.State = SyncDiverged
		info.LocalAdded = diffNames(remoteTools, localTools)
		info.RemoteAdded = diffNames(localTools, remoteTools)
	}
	return info, nil
}

// RemoteIsEmpty reports whether origin has no commits (fresh repo).
func (g *Git) RemoteIsEmpty() bool {
	_, err := g.rev("origin/main")
	return err != nil
}

// Connect links an existing local directory to a remote repo without
// cloning: init → remote add → fetch → reset to origin/main. Used when
// the environment directory already holds files (e.g. mise.toml).
// An empty remote (no commits yet) is left untouched after fetch.
func (g *Git) Connect(url string) error {
	if !g.IsRepo() {
		if err := g.Init(); err != nil {
			return err
		}
	}
	if g.RemoteURL() == "" {
		if err := g.RemoteAdd(url); err != nil {
			return err
		}
	}
	if err := g.fetch(); err != nil {
		return err
	}
	if _, err := g.rev("origin/main"); err != nil {
		return nil // remote has no commits yet
	}
	if _, err := g.run("reset", "--hard", "origin/main"); err != nil {
		return fmt.Errorf("git reset to origin/main: %w", err)
	}
	return nil
}

// RemoteAdd registers origin.
func (g *Git) RemoteAdd(url string) error {
	if _, err := g.run("remote", "add", "origin", url); err != nil {
		return fmt.Errorf("git remote add: %w", err)
	}
	return nil
}

// RemoteURL returns the origin URL ("" when absent).
func (g *Git) RemoteURL() string {
	out, err := g.run("remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (g *Git) rev(rev string) (string, error) {
	out, err := g.run("rev-parse", rev)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// showFile returns a file's content at a revision.
func (g *Git) showFile(rev, path string) (string, error) {
	out, err := g.run("show", rev+":"+path)
	if err != nil {
		return "", err
	}
	return out, nil
}

// isClean reports whether the worktree has no unstaged/staged changes.
func (g *Git) isClean() bool {
	out, err := g.run("status", "--porcelain")
	return err == nil && strings.TrimSpace(out) == ""
}

// commitAll stages everything and commits. Returns false when clean.
// Machines without a git identity get a repo-local fallback so mison
// never blocks on missing global config.
func (g *Git) commitAll(message string) (bool, error) {
	if g.isClean() {
		return false, nil
	}
	if _, err := g.run("add", "-A"); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}
	g.ensureIdentity()
	if _, err := g.run("commit", "-m", message); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	return true, nil
}

func (g *Git) ensureIdentity() {
	if _, err := g.run("config", "user.email"); err != nil {
		_, _ = g.run("config", "user.email", "mison@local")
		_, _ = g.run("config", "user.name", "mison")
	}
}

// SmartPush commits the worktree and pushes using the DESIGN.md policy:
// fetch → plain push when possible → on divergence semantic-merge onto
// origin and push the reconciliation. It returns tool names that came
// from the remote (for the mandatory ↻ notice).
func (g *Git) SmartPush(message string, resolve Resolver) ([]string, error) {
	if _, err := g.commitAll(message); err != nil {
		return nil, err
	}
	return g.sync(resolve)
}

// SmartPull fetches and applies the remote declaration. Pending local
// commits are reconciled and pushed (sync semantics: pull → apply →
// push pending).
func (g *Git) SmartPull(resolve Resolver) ([]string, error) {
	return g.sync(resolve)
}

const miseFile = "mise.toml"

func (g *Git) sync(resolve Resolver) ([]string, error) {
	if err := g.fetch(); err != nil {
		return nil, err
	}
	head, err := g.rev("HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	remote, err := g.rev("origin/main")
	if err != nil {
		// no remote branch yet — push creates it
		if err := g.push(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	mbOut, mbErr := g.run("merge-base", head, remote)
	mb := strings.TrimSpace(mbOut)
	if mbErr != nil {
		// unrelated histories (e.g. manual repo seeding): treat base as empty
		mb = ""
	}

	switch mb {
	case remote: // remote is an ancestor: local ahead (or equal)
		if head != remote {
			if err := g.push(); err != nil { // push pending commits
				return nil, err
			}
		}
		return nil, nil
	case head: // local behind: fast-forward to remote
		if _, err := g.run("reset", "--hard", remote); err != nil {
			return nil, fmt.Errorf("git reset: %w", err)
		}
		return remoteAddedTools(g, head, remote)
	}

	// diverged: semantic 3-way merge
	baseCfg, err := g.configAt(mb)
	if err != nil {
		return nil, err
	}
	localCfg, err := g.configAt(head)
	if err != nil {
		return nil, err
	}
	remoteCfg, err := g.configAt(remote)
	if err != nil {
		return nil, err
	}
	baseTools := baseCfg.Tools()
	remoteTools := remoteCfg.Tools()

	merged, conflicts := env.Merge(baseTools, localCfg.Tools(), remoteTools)
	if len(conflicts) > 0 {
		if resolve == nil {
			return nil, fmt.Errorf("conflicts on %s — rerun interactively or pass --ours/--theirs",
				strings.Join(conflictNames(conflicts), ", "))
		}
		resolved, err := resolve(conflicts)
		if err != nil {
			return nil, err
		}
		merged = append(merged, resolved...)
	}

	// rebuild the file: hard-reset worktree to remote, then write the
	// merged [tools] into the remote document and commit on top
	if _, err := g.run("reset", "--hard", remote); err != nil {
		return nil, fmt.Errorf("git reset: %w", err)
	}
	if err := writeDeclaration(g.dir, remoteCfg, merged); err != nil {
		return nil, err
	}
	if _, err := g.commitAll("mison: merge remote changes"); err != nil {
		return nil, err
	}
	if err := g.push(); err != nil {
		return nil, err
	}
	return diffNames(baseTools, remoteTools), nil
}

func (g *Git) fetch() error {
	if _, err := g.run("fetch", "origin"); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}
	return nil
}

func (g *Git) push() error {
	if _, err := g.run("push", "origin", "HEAD:main"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	return nil
}

func (g *Git) configAt(rev string) (*env.Config, error) {
	content, err := g.showFile(rev, miseFile)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "exists on disk, but not in") {
			return mustConfig("")
		}
		return nil, fmt.Errorf("git show %s:%s: %w", rev, miseFile, err)
	}
	cfg, err := env.Parse([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("parse %s:%s: %w", rev, miseFile, err)
	}
	return cfg, nil
}

// writeDeclaration rewrites the [tools] table of cfg with merged and
// returns nothing; caller then serializes cfg separately.
func writeDeclaration(dir string, cfg *env.Config, tools []env.Tool) error {
	for _, t := range cfg.Tools() {
		cfg.RemoveTool(t.Name)
	}
	for _, t := range tools {
		cfg.SetTool(t)
	}
	data, err := cfg.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, miseFile), data, 0o644)
}

func mustConfig(content string) (*env.Config, error) {
	return env.Parse([]byte(content))
}

// remoteAddedTools lists tool names the remote gained relative to the
// pre-reset local head (fast-forward case).
func remoteAddedTools(g *Git, oldHead, newRemote string) ([]string, error) {
	oldCfg, err := g.configAt(oldHead)
	if err != nil {
		return nil, err
	}
	newCfg, err := g.configAt(newRemote)
	if err != nil {
		return nil, err
	}
	return diffNames(oldCfg.Tools(), newCfg.Tools()), nil
}

// diffNames returns names present in new but not in old.
func diffNames(oldTools, newTools []env.Tool) []string {
	had := map[string]bool{}
	for _, t := range oldTools {
		had[t.Name] = true
	}
	var added []string
	for _, t := range newTools {
		if !had[t.Name] {
			added = append(added, t.Name)
		}
	}
	return added
}

func conflictNames(cs []env.Conflict) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name
	}
	return names
}
