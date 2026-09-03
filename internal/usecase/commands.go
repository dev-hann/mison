package usecase

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dev-hann/mison/internal/detector"
	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/lockfile"
	"github.com/dev-hann/mison/internal/paths"
	"github.com/dev-hann/mison/internal/repo/miserepo"
	"github.com/dev-hann/mison/internal/ui"
)

// GhClient is the gh surface flows depend on.
type GhClient interface {
	IsInstalled() bool
	AuthStatus() bool
	AuthLogin() error
	SetupGit() error
	RepoExists(name string) bool
	RepoURL(name string) (string, error)
	CreatePrivateRepo(name string) (string, error)
}

// DefaultRepoName is the environment repository mison creates.
const DefaultRepoName = "mison-env"

// repoConfigPath is the local-only file remembering which env repo
// this machine is bound to (guards against default-name drift across
// mison versions). Never committed to the env repo.
func (f *Flows) repoConfigPath() string {
	return filepath.Join(f.Home, ".mison", "config.toml")
}

// resolveRepoName picks the repo name: explicit flag > persisted >
// default. A successful connection persists the choice.
func (f *Flows) resolveRepoName(flagged string) string {
	if flagged != "" {
		return flagged
	}
	if data, err := os.ReadFile(f.repoConfigPath()); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "repo = "); ok {
				return strings.Trim(v, `"`)
			}
		}
	}
	return DefaultRepoName
}

func (f *Flows) persistRepoName(name string) {
	if name == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(f.repoConfigPath()), 0o755); err != nil {
		return
	}
	content := "# managed by mison\nrepo = " + strconv.Quote(name) + "\n"
	_ = os.WriteFile(f.repoConfigPath(), []byte(content), 0o644)
}

// Flows carries the dependencies for mison's command flows. All fields
// are injected; tests provide fakes. Flows reach the user exclusively
// through the two interaction ports (UI, Ask).
type Flows struct {
	Home string
	UI   Reporter
	Ask  Prompter
	Mise MiseRepoIface
	Look detector.LookPathFunc
	Git  func(dir string) EnvRepoIface
	Gh   GhClient
}

// MiseRepoIface is the mise surface flows depend on.
type MiseRepoIface interface {
	IsInstalled() bool
	RunInstaller() error
	Exec(args ...string) error
	ListInstalled() ([]miserepo.Entry, error)
}

// EnvRepoIface is the environment-repository surface flows depend on
// (satisfied by *Engine).
type EnvRepoIface interface {
	IsRepo() bool
	Init() error
	Connect(url string) error
	RemoteAdd(url string) error
	RemoteSetURL(url string) error
	RemoteURL() string
	RemoteIsEmpty() bool
	SyncStatus() (SyncInfo, error)
	SmartPush(message string, resolve Resolver) ([]string, error)
	SmartPull(resolve Resolver) ([]string, error)
}

func (f *Flows) layout() paths.Layout { return paths.New(f.Home) }

// acquireRunLock refuses to run while another mison process on this
// machine holds the run lock (prevents git index corruption from two
// concurrent syncs). Kernel-released: crashed runs never block.
func (f *Flows) acquireRunLock() (*lockfile.Guard, error) {
	return lockfile.Acquire(f.layout().RunLock)
}
func (f *Flows) detect() detector.Info {
	return detector.Detect()
}

func (f *Flows) envRepo() EnvRepoIface { return f.Git(f.layout().EnvDir) }

// installedTools returns the ownership-filtered active tools.
func (f *Flows) installedTools() ([]env.Tool, error) {
	entries, err := f.Mise.ListInstalled()
	if err != nil {
		return nil, err
	}
	return OwnedTools(entries, f.Home), nil
}

func (f *Flows) ensureMise() error {
	if detector.IsMiseInstalled(f.Look) {
		return nil
	}
	f.UI.Step("Installing mise")
	return f.Mise.RunInstaller()
}

func (f *Flows) loadConfig() (*env.Config, error) {
	data, err := os.ReadFile(f.layout().MiseToml)
	if err != nil {
		return nil, fmt.Errorf("read mise.toml: %w", err)
	}
	return env.Parse(data)
}

func (f *Flows) saveConfig(cfg *env.Config) error {
	if err := cfg.StampSchema(); err != nil {
		return err
	}
	data, err := cfg.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(f.layout().MiseToml, data, 0o644); err != nil {
		return fmt.Errorf("write mise.toml: %w", err)
	}
	return nil
}

// makeResolver builds a Resolver from a policy and the Prompter.
func (f *Flows) makeResolver(policy ConflictPolicy) Resolver {
	return func(conflicts []env.Conflict) ([]env.Tool, error) {
		out := make([]env.Tool, 0, len(conflicts))
		for _, c := range conflicts {
			switch policy {
			case PolicyOurs:
				out = append(out, pickSide(c.Local, c.Remote))
			case PolicyTheirs:
				out = append(out, pickSide(c.Remote, c.Local))
			default:
				tool, err := f.Ask.ResolveConflict(c)
				if err != nil {
					return nil, err
				}
				out = append(out, tool)
			}
		}
		return out, nil
	}
}

// refreshLock regenerates the lockfile and reports whether the env
// repo's lock content changed. `mise lock --global` writes via atomic
// rename, which REPLACES the ~/.config/mise/mise.lock symlink with a
// regular file (verified against mise 2026.9.1) — so mison adopts the
// freshly written content into the env repo and restores the symlink.
// The env repo stays the single source of truth. Lock is derived
// state — best-effort, never blocks the flow.
func (f *Flows) refreshLock() bool {
	l := f.layout()
	before, _ := os.ReadFile(l.MiseLock)
	if err := f.Mise.Exec("lock", "--global"); err != nil {
		f.UI.Warn("could not refresh lockfile — will retry on next sync (" + err.Error() + ")")
		return false
	}
	if info, err := os.Lstat(l.GlobalLock); err == nil && info.Mode()&os.ModeSymlink == 0 {
		if data, readErr := os.ReadFile(l.GlobalLock); readErr == nil {
			if writeErr := os.WriteFile(l.MiseLock, data, 0o644); writeErr == nil {
				_ = os.Remove(l.GlobalLock)
				_ = os.Symlink(l.MiseLock, l.GlobalLock)
			}
		}
	}
	after, _ := os.ReadFile(l.MiseLock)
	return !bytes.Equal(before, after)
}

// pushErrHint extends push/pull failure warnings with the recovery
// path implied by the error text (heuristic — both hints are safe to
// show even on a misread).
func pushErrHint(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist"):
		return " — the env repo may be gone; re-bind with mison init --repo <name>"
	case strings.Contains(msg, "authentication") || strings.Contains(msg, "403") ||
		strings.Contains(msg, "could not read Username"):
		return " — GitHub auth expired; run mison init (gh auth login)"
	}
	return ""
}

// commitAndPush applies the push policy after a declaration change:
// no repo → skip silently; divergence → reconcile; offline → warn and
// defer to the next sync. Future-schema remotes are fatal — the local
// declaration was saved, but nothing may be reset or pushed.
func (f *Flows) commitAndPush(message string, policy ConflictPolicy) error {
	repo := f.envRepo()
	if !repo.IsRepo() {
		return nil
	}
	merged, err := repo.SmartPush(message, f.makeResolver(policy))
	if err != nil {
		if errors.Is(err, env.ErrFutureSchema) {
			f.UI.Fail("declaration saved locally — push refused: " + err.Error())
			return err
		}
		warn := "could not push — will retry on next sync (" + err.Error() + ")" + pushErrHint(err)
		f.UI.Warn(warn)
		return nil
	}
	if len(merged) > 0 {
		f.UI.Synced(fmt.Sprintf("Remote had new changes (%s) — merged automatically", strings.Join(merged, ", ")))
		// the merge path hard-resets to the remote, clobbering the lock
		// regenerated above — regenerate once more and push when the
		// content differs (bounded: one extra round, no recursion)
		if f.refreshLock() {
			if _, err := repo.SmartPush("mison: refresh lock", f.makeResolver(policy)); err != nil {
				f.UI.Warn("could not push lockfile — will retry on next sync (" + err.Error() + ")")
			}
		}
		return nil
	}
	return nil
}

func tool(name, version string) env.Tool {
	return env.Tool{Name: name, Version: version}
}

// RunInstall implements the install flow: declare tools, apply them,
// verify visibility, push the declaration.
func (f *Flows) RunInstall(args []string, osFlag string, policy ConflictPolicy) error {
	guard, err := f.acquireRunLock()
	if err != nil {
		f.UI.Fail(err.Error())
		return err
	}
	defer guard.Release()

	osSpec := env.ParseOSSpec(osFlag)
	if osFlag != "" && osSpec == nil {
		return fmt.Errorf("invalid OS spec %q (use mac, linux, linux/x64, linux/arm64, macos/arm64)", osFlag)
	}

	if _, err := f.layout().Ensure(); err != nil {
		return err
	}
	if err := f.ensureMise(); err != nil {
		return err
	}

	cfg, err := f.loadConfig()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(args))
	var tools []env.Tool
	for _, spec := range args {
		name, version, err := env.ParseToolSpec(spec)
		if err != nil {
			return err
		}
		t := env.Tool{Name: name, Version: version, OS: osSpec}
		cfg.SetTool(t)
		tools = append(tools, t)
		names = append(names, name)
	}
	if err := f.saveConfig(cfg); err != nil {
		return err
	}

	info := f.detect()
	skipped := map[string]bool{}
	for _, t := range tools {
		if len(t.OS) > 0 && !t.AppliesTo(info.OS, info.Arch) {
			skipped[t.Name] = true
			f.UI.Warn(fmt.Sprintf("%s: restricted to %s — skipped on this machine (%s/%s)",
				t.Name, strings.Join(t.OS, ", "), info.OS, info.Arch))
		}
	}

	f.UI.Step(fmt.Sprintf("Installing %s", strings.Join(names, ", ")))
	if err := f.Mise.Exec("install"); err != nil {
		return fmt.Errorf("%w — declaration saved; drop broken tools with mison uninstall <tool>", err)
	}
	f.verifyVisible(names, skipped)
	f.refreshLock()
	return f.commitAndPush(fmt.Sprintf("install: %s", strings.Join(names, ", ")), policy)
}

// verifyVisible warns when tools mison just installed are not reported
// by mise — catches silent no-ops (e.g. broken global-config symlink).
func (f *Flows) verifyVisible(names []string, ignore map[string]bool) {
	installed, err := f.installedTools()
	if err != nil {
		f.UI.Warn("could not verify installation (" + err.Error() + ")")
		return
	}
	present := map[string]bool{}
	for _, t := range installed {
		present[t.Name] = true
	}
	for _, name := range names {
		if ignore[name] || present[name] {
			continue
		}
		f.UI.Warn(fmt.Sprintf("%s not visible to mise — declaration saved; run mison status to check", name))
	}
}

// verifyDeclaredApplied re-checks the declaration after sync applied
// it; OS-restricted tools that do not target this machine are exempt.
func (f *Flows) verifyDeclaredApplied(declared []env.Tool) {
	installed, err := f.installedTools()
	if err != nil {
		f.UI.Warn("could not verify installation (" + err.Error() + ")")
		return
	}
	info := f.detect()
	present := make(map[string]bool, len(installed))
	for _, t := range installed {
		present[t.Name] = true
	}
	for _, d := range declared {
		if !d.AppliesTo(info.OS, info.Arch) {
			continue
		}
		if !present[d.Name] {
			f.UI.Warn(fmt.Sprintf("%s not visible to mise — run mison status to check", d.Name))
		}
	}
}

// RunUninstall implements the uninstall flow.
func (f *Flows) RunUninstall(args []string, assumeYes bool, policy ConflictPolicy) error {
	guard, err := f.acquireRunLock()
	if err != nil {
		f.UI.Fail(err.Error())
		return err
	}
	defer guard.Release()

	if !assumeYes && !f.Ask.Confirm(fmt.Sprintf("Remove %s from the environment?", strings.Join(args, ", "))) {
		return nil
	}

	if _, err := f.layout().Ensure(); err != nil {
		return err
	}
	if err := f.ensureMise(); err != nil {
		return err
	}

	cfg, err := f.loadConfig()
	if err != nil {
		return err
	}

	installed := map[string]bool{}
	if tools, err := f.installedTools(); err == nil {
		for _, t := range tools {
			installed[t.Name] = true
		}
	}

	var missing []string
	for _, name := range args {
		if !cfg.RemoveTool(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("not in environment: %s", strings.Join(missing, ", "))
	}
	if err := f.saveConfig(cfg); err != nil {
		return err
	}

	for _, name := range args {
		if installed[name] {
			f.UI.Step(fmt.Sprintf("Removing %s", name))
			if err := f.Mise.Exec("uninstall", "--all", name); err != nil {
				return err
			}
		} else {
			f.UI.Step(fmt.Sprintf("Removed %s (not installed)", name))
		}
	}
	f.refreshLock()
	return f.commitAndPush(fmt.Sprintf("uninstall: %s", strings.Join(args, ", ")), policy)
}

// RunStatus implements the status flow (read-only).
func (f *Flows) RunStatus() error {
	if _, err := os.Stat(f.layout().MiseToml); err != nil {
		return fmt.Errorf("no environment found — run mison init first")
	}
	cfg, err := f.loadConfig()
	if err != nil {
		return err
	}
	installed, err := f.installedTools()
	if err != nil {
		return err
	}

	declared := cfg.Tools()
	diff := env.Diff(declared, installed)

	r := f.UI
	r.Line("Environment status")
	f.renderSyncStatus()
	var missing int
	for _, st := range diff {
		switch st.State {
		case env.StateOK:
			r.ToolLine(ui.MarkOK, st.Tool.Name, st.Tool.Version)
		case env.StateMissing:
			r.ToolLine(ui.MarkFail, st.Tool.Name, "missing — run mison sync")
			missing++
		case env.StateMismatch:
			r.ToolLine(ui.MarkWarning, st.Tool.Name,
				fmt.Sprintf("declared %s, installed %s — run mison sync", st.Tool.Version, st.Installed))
		}
	}
	if len(diff) == 0 {
		r.Line("No tools declared.")
	}
	if missing > 0 {
		r.Warn(fmt.Sprintf("%d tool(s) missing", missing))
	}
	return nil
}

// renderSyncStatus reports the local-vs-GitHub declaration relation.
func (f *Flows) renderSyncStatus() {
	r := f.UI
	repo := f.envRepo()
	if !repo.IsRepo() {
		r.Line("Sync: not connected — run mison init to link GitHub")
		return
	}
	info, err := repo.SyncStatus()
	if err != nil {
		r.Warn("could not compare with GitHub (" + err.Error() + ")")
		return
	}
	switch info.State {
	case SyncUpToDate:
		r.Step("up to date with GitHub")
	case SyncBehind:
		r.ToolLine(ui.MarkSync, "remote has new tools", strings.Join(info.RemoteAdded, ", ")+" — run mison sync")
	case SyncAhead:
		r.ToolLine(ui.MarkWarning, "local changes not pushed", "run mison sync")
	case SyncDiverged:
		r.ToolLine(ui.MarkWarning, "diverged from GitHub", "local and remote both changed — run mison sync")
	}
}

// RunSync implements the sync flow: pull declaration (when connected),
// apply it via mise, prune orphans on request.
func (f *Flows) RunSync(prune bool, policy ConflictPolicy) error {
	guard, err := f.acquireRunLock()
	if err != nil {
		f.UI.Fail(err.Error())
		return err
	}
	defer guard.Release()

	if _, err := os.Stat(f.layout().MiseToml); err != nil {
		return fmt.Errorf("no environment found — run mison init first")
	}
	// restore the global-config symlink: machines that received the env
	// by cloning (or lost the symlink) must still be seen by mise
	if _, err := f.layout().Ensure(); err != nil {
		return err
	}
	if err := f.ensureMise(); err != nil {
		return err
	}

	if repo := f.envRepo(); repo.IsRepo() {
		f.UI.Step("Pulling environment")
		merged, err := repo.SmartPull(f.makeResolver(policy))
		switch {
		case errors.Is(err, env.ErrFutureSchema):
			return err
		case err != nil:
			f.UI.Warn("pull failed — continuing with local declaration (" + err.Error() + ")" + pushErrHint(err))
		case len(merged) > 0:
			f.UI.Synced(fmt.Sprintf("New changes: %s", strings.Join(merged, ", ")))
		}
	}

	cfg, err := f.loadConfig()
	if err != nil {
		return err
	}
	declared := cfg.Tools()
	installed, err := f.installedTools()
	if err != nil {
		return err
	}

	diff := env.Diff(declared, installed)
	var needsApply bool
	for _, st := range diff {
		if st.State != env.StateOK {
			needsApply = true
			break
		}
	}
	if needsApply {
		f.UI.Step("Installing declared tools")
		if err := f.Mise.Exec("install"); err != nil {
			return fmt.Errorf("%w — drop broken tools with mison uninstall <tool>", err)
		}
		f.verifyDeclaredApplied(declared)
	}

	declaredNames := map[string]bool{}
	for _, t := range declared {
		declaredNames[t.Name] = true
	}
	var orphans []string
	for _, t := range installed {
		if t.Name == "gh" {
			// gh is mison's bootstrap tool (auth + push) — never offer
			// it for orphan removal; explicit `mison uninstall gh` stays
			// available for users who really mean it
			continue
		}
		if !declaredNames[t.Name] {
			orphans = append(orphans, t.Name)
		}
	}
	sort.Strings(orphans)

	r := f.UI
	removeOrphan := func(name string) error {
		r.Step(fmt.Sprintf("Pruning %s", name))
		return f.Mise.Exec("uninstall", "--all", name)
	}
	pruneAll := func() error {
		var failed []string
		for _, name := range orphans {
			if err := removeOrphan(name); err != nil {
				failed = append(failed, name+": "+err.Error())
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("pruned with failures: %s", strings.Join(failed, "; "))
		}
		return nil
	}
	switch {
	case len(orphans) == 0:
		// nothing extra
	case prune:
		return pruneAll()
	default:
		if f.Ask.Confirm(fmt.Sprintf("Remove undeclared tools (%s)?", strings.Join(orphans, ", "))) {
			return pruneAll()
		}
		r.Warn("kept (run mison sync --prune to remove automatically)")
	}

	// lock is derived from the declaration: regenerate only when the
	// declaration was applied here (skip the registry round-trip on
	// no-op syncs), then push when the content actually changed
	if needsApply && f.refreshLock() {
		r.Step("Refreshing lockfile")
		if err := f.commitAndPush("mison: refresh lock", policy); err != nil {
			return err
		}
	}

	if needsApply || len(orphans) > 0 {
		r.Step("Environment synchronized")
	} else {
		r.Step("Already synchronized")
	}
	return nil
}

// RunInit implements the init flow: bootstrap this machine into a
// mison environment (mise → gh → private env repo → declaration
// symlink) and install the declared tools.
func (f *Flows) RunInit(repoName string) error {
	guard, err := f.acquireRunLock()
	if err != nil {
		f.UI.Fail(err.Error())
		return err
	}
	defer guard.Release()

	r := f.UI
	info := f.detect()
	r.Step(fmt.Sprintf("Detected %s/%s", info.OS, info.Arch))

	if _, err := f.layout().Ensure(); err != nil {
		return err
	}
	if err := f.ensureMise(); err != nil {
		return err
	}

	if err := f.ensureGh(); err != nil {
		return err
	}

	explicit := repoName != ""
	repoName = f.resolveRepoName(repoName)
	if err := f.connectRepo(repoName, explicit); err != nil {
		return err
	}
	f.persistRepoName(repoName)

	r.Step("Installing declared tools")
	if err := f.Mise.Exec("install"); err != nil {
		return err
	}
	if f.refreshLock() {
		r.Step("Refreshing lockfile")
		if err := f.commitAndPush("mison: refresh lock", PolicyAsk); err != nil {
			return err
		}
	}
	r.Step("Environment ready")
	return nil
}

// ensureGh installs gh via mise, declares it in mise.toml (gh is part
// of the environment so every machine bootstraps itself), then
// authenticates and wires git credentials.
func (f *Flows) ensureGh() error {
	if !f.Gh.IsInstalled() {
		f.UI.Step("Installing gh")
		if err := f.Mise.Exec("install", "gh@latest"); err != nil {
			return err
		}
	}

	cfg, err := f.loadConfig()
	if err != nil {
		return err
	}
	ghDeclared := false
	for _, t := range cfg.Tools() {
		if t.Name == "gh" {
			ghDeclared = true
		}
	}
	if !ghDeclared {
		cfg.SetTool(tool("gh", "latest"))
		if err := f.saveConfig(cfg); err != nil {
			return err
		}
	}

	if !f.Gh.AuthStatus() {
		f.UI.Line("GitHub login required — follow the browser prompt:")
		if err := f.Gh.AuthLogin(); err != nil {
			return err
		}
	}
	return f.Gh.SetupGit()
}

// connectExisting links to a repo another machine already created:
// fetch+reset (or seed push when the remote is still empty).
func (f *Flows) connectExisting(repo EnvRepoIface, repoName string) error {
	f.UI.Step("Connecting to existing environment repository " + repoName)
	url, err := f.Gh.RepoURL(repoName)
	if err != nil {
		return err
	}
	if err := repo.Connect(url); err != nil {
		return err
	}
	if repo.RemoteIsEmpty() {
		// remote created but never seeded: push the initial state
		_, err = repo.SmartPush("mison: init environment", f.makeResolver(PolicyAsk))
	}
	return err
}

// connectRepo links ~/.mison/env to the GitHub environment repo:
// local clone → smart pull (explicit --repo re-binds first when the
// target differs); remote exists (another machine created it) →
// connect by fetch+reset; otherwise create the private repo, init
// git, and push the initial declaration. A create race (another
// machine created the repo between our exists-check and the create)
// falls back to connecting.
func (f *Flows) connectRepo(repoName string, explicit bool) error {
	repo := f.envRepo()

	if repo.IsRepo() && repo.RemoteURL() != "" {
		if explicit {
			if url, err := f.Gh.RepoURL(repoName); err == nil && url != "" && url != repo.RemoteURL() {
				f.UI.Step("Re-binding to " + repoName)
				if err := repo.RemoteSetURL(url); err != nil {
					return err
				}
			}
		}
		f.UI.Step("Connecting environment")
		_, err := repo.SmartPull(f.makeResolver(PolicyAsk))
		return err
	}

	if f.Gh.RepoExists(repoName) {
		return f.connectExisting(repo, repoName)
	}

	f.UI.Step("Creating environment repository " + repoName)
	url, err := f.Gh.CreatePrivateRepo(repoName)
	if err != nil {
		if f.Gh.RepoExists(repoName) {
			// another machine won the create race — connect instead
			return f.connectExisting(repo, repoName)
		}
		return err
	}

	if err := repo.Init(); err != nil {
		return err
	}
	if err := repo.RemoteAdd(url); err != nil {
		return err
	}
	_, err = repo.SmartPush("mison: init environment", f.makeResolver(PolicyAsk))
	return err
}
