package usecase

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/repo/gitrepo"
	"github.com/dev-hann/mison/internal/repo/miserepo"
	"github.com/dev-hann/mison/internal/xdg"
)

// Resolver settles same-tool conflicts produced by the semantic merge.
// Return the winning tools (empty Tool = removal wins on that side).
type Resolver func(conflicts []env.Conflict) ([]env.Tool, error)

const miseFile = "mise.toml"

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

// SyncAction is the operation a sync step must perform.
type SyncAction int

// Sync actions.
const (
	ActionSeedRemote  SyncAction = iota // remote has no commits yet
	ActionPush                          // local ahead or equal
	ActionFastForward                   // local behind
	ActionMerge                         // diverged
)

// SyncPlan is the pure decision for one sync step.
type SyncPlan struct {
	Action SyncAction
}

// PlanSync decides the sync action from revision relations. base is
// the merge-base of head and remote ("" means unrelated histories).
// hasRemote reports whether origin/main exists.
func PlanSync(head, remote, base string, hasRemote bool) SyncPlan {
	if !hasRemote {
		return SyncPlan{Action: ActionSeedRemote}
	}
	if head == remote {
		return SyncPlan{Action: ActionPush} // no-op push
	}
	switch base {
	case remote:
		return SyncPlan{Action: ActionPush}
	case head:
		return SyncPlan{Action: ActionFastForward}
	default:
		return SyncPlan{Action: ActionMerge}
	}
}

// Engine implements the DESIGN.md §3 sync policy (fetch first, semantic
// 3-way merge, never a conflicted git state) over atomic git commands.
type Engine struct {
	git *gitrepo.Repo
}

// NewEngine builds the sync engine for a working directory.
func NewEngine(git *gitrepo.Repo) *Engine { return &Engine{git: git} }

// Dir returns the working directory.
func (e *Engine) Dir() string { return e.git.Dir() }

// IsRepo reports whether the directory is a git repository.
func (e *Engine) IsRepo() bool { return e.git.IsRepo() }

// Init creates a fresh git repository.
func (e *Engine) Init() error { return e.git.Init() }

// RemoteAdd registers origin.
func (e *Engine) RemoteAdd(url string) error { return e.git.RemoteAdd(url) }

// RemoteURL returns the origin URL ("" when absent).
func (e *Engine) RemoteURL() string { return e.git.RemoteURL() }

// RemoteIsEmpty reports whether origin has no commits (fresh repo).
func (e *Engine) RemoteIsEmpty() bool {
	_, err := e.git.Rev("origin/main")
	return err != nil
}

// Connect links an existing local directory to a remote repo without
// cloning: init → remote add → fetch → reset to origin/main. Used when
// the environment directory already holds files. An empty remote is
// left untouched after fetch.
func (e *Engine) Connect(url string) error {
	if !e.IsRepo() {
		if err := e.git.Init(); err != nil {
			return err
		}
	}
	if e.RemoteURL() == "" {
		if err := e.git.RemoteAdd(url); err != nil {
			return err
		}
	}
	if err := e.git.Fetch(); err != nil {
		return err
	}
	if e.RemoteIsEmpty() {
		return nil
	}
	return e.git.ResetHard("origin/main")
}

// commitAll stages everything and commits; false when the tree is
// clean. Machines without a git identity get a repo-local fallback.
func (e *Engine) commitAll(message string) (bool, error) {
	if e.git.IsClean() {
		return false, nil
	}
	if err := e.git.StageAll(); err != nil {
		return false, err
	}
	if !e.git.HasIdentity() {
		if err := e.git.SetIdentityFallback(); err != nil {
			return false, err
		}
	}
	if err := e.git.Commit(message); err != nil {
		return false, err
	}
	return true, nil
}

// SmartPush commits the worktree and pushes using the DESIGN.md
// policy. It returns tool names that came from the remote (for the
// mandatory ↻ notice).
func (e *Engine) SmartPush(message string, resolve Resolver) ([]string, error) {
	if _, err := e.commitAll(message); err != nil {
		return nil, err
	}
	return e.sync(resolve)
}

// SmartPull fetches and applies the remote declaration. Pending local
// commits are reconciled and pushed (sync semantics).
func (e *Engine) SmartPull(resolve Resolver) ([]string, error) {
	return e.sync(resolve)
}

func (e *Engine) sync(resolve Resolver) ([]string, error) {
	// DESIGN.md Case F: manual edits are auto-committed before any pull
	// or reset, so nothing is ever destroyed by a reconciliation.
	if _, err := e.commitAll("mison: manual changes"); err != nil {
		return nil, err
	}
	if err := e.git.Fetch(); err != nil {
		return nil, err
	}

	head, err := e.git.Rev("HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	remote, remoteErr := e.git.Rev("origin/main")
	base := ""
	if remoteErr == nil {
		if out, mbErr := e.git.MergeBase(head, remote); mbErr == nil {
			base = out
		}
	}

	switch PlanSync(head, remote, base, remoteErr == nil).Action {
	case ActionSeedRemote:
		if err := e.git.Push(); err != nil {
			return nil, err
		}
		return nil, nil

	case ActionPush:
		if head != remote { // push pending commits
			if err := e.git.Push(); err != nil {
				return nil, err
			}
		}
		return nil, nil

	case ActionFastForward:
		// parse remote BEFORE resetting so a bad remote never touches
		// the worktree
		added, err := e.remoteAdded(head, remote)
		if err != nil {
			return nil, err
		}
		if err := e.git.ResetHard(remote); err != nil {
			return nil, err
		}
		return added, nil
	}

	// ActionMerge: semantic 3-way merge
	baseCfg, err := e.configAt(base)
	if err != nil {
		return nil, err
	}
	localCfg, err := e.configAt(head)
	if err != nil {
		return nil, err
	}
	remoteCfg, err := e.configAt(remote)
	if err != nil {
		return nil, err
	}
	baseTools, localTools, remoteTools := baseCfg.Tools(), localCfg.Tools(), remoteCfg.Tools()

	merged, conflicts := env.Merge(baseTools, localTools, remoteTools)
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

	// hard-reset worktree to remote, then write the merged [tools] into
	// the remote document and commit on top
	if err := e.git.ResetHard(remote); err != nil {
		return nil, err
	}
	if err := e.rebuild(remoteCfg, merged); err != nil {
		return nil, err
	}
	if _, err := e.commitAll("mison: merge remote changes"); err != nil {
		return nil, err
	}
	if err := e.git.Push(); err != nil {
		return nil, err
	}
	return diffNames(baseTools, remoteTools), nil
}

// SyncStatus fetches and compares the local declaration with the
// remote without modifying anything.
func (e *Engine) SyncStatus() (SyncInfo, error) {
	var info SyncInfo
	if err := e.git.Fetch(); err != nil {
		return info, err
	}
	head, err := e.git.Rev("HEAD")
	if err != nil {
		return info, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	remote, err := e.git.Rev("origin/main")
	if err != nil {
		return info, fmt.Errorf("no origin/main branch: %w", err)
	}
	if head == remote {
		info.State = SyncUpToDate
		return info, nil
	}

	base := ""
	if out, mbErr := e.git.MergeBase(head, remote); mbErr == nil {
		base = out
	}
	localCfg, err := e.configAt(head)
	if err != nil {
		return info, err
	}
	remoteCfg, err := e.configAt(remote)
	if err != nil {
		return info, err
	}
	localTools, remoteTools := localCfg.Tools(), remoteCfg.Tools()

	switch PlanSync(head, remote, base, true).Action {
	case ActionPush:
		info.State = SyncAhead
		info.LocalAdded = diffNames(remoteTools, localTools)
	case ActionFastForward:
		info.State = SyncBehind
		info.RemoteAdded = diffNames(localTools, remoteTools)
	default:
		info.State = SyncDiverged
		info.LocalAdded = diffNames(remoteTools, localTools)
		info.RemoteAdded = diffNames(localTools, remoteTools)
	}
	return info, nil
}

// remoteAdded lists tool names the remote gained relative to oldHead.
func (e *Engine) remoteAdded(oldHead, newRemote string) ([]string, error) {
	oldCfg, err := e.configAt(oldHead)
	if err != nil {
		return nil, err
	}
	newCfg, err := e.configAt(newRemote)
	if err != nil {
		return nil, err
	}
	return diffNames(oldCfg.Tools(), newCfg.Tools()), nil
}

func (e *Engine) configAt(rev string) (*env.Config, error) {
	content, err := e.git.ShowFile(rev, miseFile)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") ||
			strings.Contains(err.Error(), "exists on disk, but not in") {
			return env.Parse([]byte(""))
		}
		return nil, err
	}
	cfg, err := env.Parse([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("parse %s:%s: %w", rev, miseFile, err)
	}
	return cfg, nil
}

// rebuild rewrites the [tools] table of cfg with the merged tool set
// and writes the serialized document into the working directory.
func (e *Engine) rebuild(cfg *env.Config, tools []env.Tool) error {
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
	return os.WriteFile(filepath.Join(e.git.Dir(), miseFile), data, 0o644)
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

// OwnedTools filters raw mise entries down to the active tools declared
// by mison's own config (the global symlink or the ~/.mison/env
// declaration). Project configs and foreign globals are excluded.
func OwnedTools(entries []miserepo.Entry, home string) []env.Tool {
	ours := ownedPaths(home)
	seen := map[string]bool{}
	var tools []env.Tool
	for _, e := range entries {
		if !e.Active || seen[e.Name] || !ours.has(e.Source) {
			continue
		}
		seen[e.Name] = true
		tools = append(tools, env.Tool{Name: e.Name, Version: e.Version})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

// pathSet matches config paths lexically and through symlink resolution.
type pathSet map[string]struct{}

func (p pathSet) has(path string) bool {
	clean := filepath.Clean(path)
	if _, ok := p[clean]; ok {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		_, ok := p[resolved]
		return ok
	}
	return false
}

func ownedPaths(home string) pathSet {
	global := filepath.Join(xdg.ConfigDir(home), "mise", "config.toml")
	decl := filepath.Join(home, ".mison", "env", "mise.toml")

	set := pathSet{}
	for _, p := range []string{global, decl} {
		set[filepath.Clean(p)] = struct{}{}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			set[resolved] = struct{}{}
		}
	}
	return set
}

// pickSide prefers the primary side, falling back to the other when
// the primary is a removal (empty tool).
func pickSide(primary, fallback env.Tool) env.Tool {
	if primary.Name == "" && fallback.Name != "" {
		return fallback
	}
	return primary
}

// PickSide is the exported conflict-side selector for adapters
// (TermUI prompts) that need the same rule.
func PickSide(primary, fallback env.Tool) env.Tool { return pickSide(primary, fallback) }
