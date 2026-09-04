package usecase

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/lockfile"
	"github.com/dev-hann/mison/internal/repo/miserepo"
)

// fakeMise implements MiseRepoIface; entries default to mison-owned
// sources so OwnedTools passes them through.
type fakeMise struct {
	home           string
	extra          []miserepo.Entry // foreign/inactive extras
	execCalls      []string
	installDone    bool
	execErr        error
	listErr        error
	execFailArg    string
	bumpCandidates []miserepo.BumpCandidate
	// lockResult: when set, Exec("lock", "--global") writes it to the
	// env-dir lockfile — simulates mise's side effect through the symlink.
	// lockResults (when set) supplies DIFFERENT content per successive
	// lock call (last entry repeats) — simulates a merge clobbering the
	// lock between regenerations.
	lockResult  string
	lockResults []string
	lockCalls   int
}

func (f *fakeMise) setInstalled(pairs ...string) {
	f.extra = nil
	for i := 0; i+1 < len(pairs); i += 2 {
		f.extra = append(f.extra, miserepo.Entry{
			Name: pairs[i], Version: pairs[i+1], Active: true,
			Source: filepath.Join(f.home, ".config", "mise", "config.toml"),
		})
	}
}

func (f *fakeMise) IsInstalled() bool { return true }
func (f *fakeMise) RunInstaller() error {
	f.installDone = true
	return nil
}
func (f *fakeMise) Exec(args ...string) error {
	if f.execErr != nil {
		return f.execErr
	}
	joined := strings.Join(args, " ")
	if f.execFailArg != "" && strings.Contains(joined, f.execFailArg) {
		return fmt.Errorf("uninstall failed: %s", f.execFailArg)
	}
	if joined == "lock --global" {
		content := f.lockResult
		if len(f.lockResults) > 0 {
			i := f.lockCalls
			if i >= len(f.lockResults) {
				i = len(f.lockResults) - 1
			}
			content = f.lockResults[i]
		}
		f.lockCalls++
		if content != "" {
			// mimic real mise: atomic rename REPLACES the global-lock
			// symlink with a regular file (content never lands in the
			// env repo through the link)
			global := filepath.Join(f.home, ".config", "mise", "mise.lock")
			_ = os.Remove(global)
			if err := os.WriteFile(global, []byte(content), 0o644); err != nil {
				return err
			}
		}
	}
	f.execCalls = append(f.execCalls, joined)
	return nil
}
func (f *fakeMise) BumpDryRun() ([]miserepo.BumpCandidate, error) {
	return f.bumpCandidates, nil
}

func (f *fakeMise) ListInstalled() ([]miserepo.Entry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.extra, nil
}

func foundLookPath(string) (string, error) { return "/usr/bin/mise", nil }

type fakeRepo struct {
	isRepo      bool
	remote      string
	pushes      []string
	pulls       int
	pushErr     error
	pullErr     error
	mergedOn    []string
	connected   []string
	setURLs     []string
	remoteEmpty bool
	syncState   SyncState
	remoteAdded []string
	syncErr     error
	conflict    *env.Conflict
}

func (f *fakeRepo) IsRepo() bool { return f.isRepo }
func (f *fakeRepo) Init() error  { f.isRepo = true; return nil }
func (f *fakeRepo) RemoteAdd(url string) error {
	f.remote = url
	return nil
}
func (f *fakeRepo) RemoteSetURL(url string) error {
	f.setURLs = append(f.setURLs, url)
	f.remote = url
	return nil
}
func (f *fakeRepo) RemoteURL() string { return f.remote }
func (f *fakeRepo) SmartPush(message string, resolve Resolver) ([]string, error) {
	if f.pushErr != nil {
		return nil, f.pushErr
	}
	if f.conflict != nil && resolve != nil {
		if _, err := resolve([]env.Conflict{*f.conflict}); err != nil {
			return nil, err
		}
	}
	f.pushes = append(f.pushes, message)
	return f.mergedOn, nil
}
func (f *fakeRepo) SmartPull(_ Resolver) ([]string, error) {
	if f.pullErr != nil {
		return nil, f.pullErr
	}
	f.pulls++
	return f.mergedOn, nil
}
func (f *fakeRepo) Connect(url string) error {
	f.connected = append(f.connected, url)
	f.isRepo = true
	f.remote = url
	return nil
}
func (f *fakeRepo) RemoteIsEmpty() bool { return f.remoteEmpty }
func (f *fakeRepo) SyncStatus() (SyncInfo, error) {
	return SyncInfo{State: f.syncState, RemoteAdded: f.remoteAdded}, f.syncErr
}

type fakeGh struct {
	installed    bool
	authed       bool
	created      []string
	exists       bool
	url          string
	createErr    error
	whoami       string
	whoamiErr    error
	latestTag    string
	installerRun bool
	// existsFlip: when set, the first RepoExists call returns false and
	// later calls return exists — simulates a create race.
	existsFlip bool
	existsSeen int
}

func (f *fakeGh) IsInstalled() bool       { return f.installed }
func (f *fakeGh) AuthStatus() bool        { return f.authed }
func (f *fakeGh) Whoami() (string, error) { return f.whoami, f.whoamiErr }
func (f *fakeGh) AuthLogin() error        { f.authed = true; return nil }
func (f *fakeGh) SetupGit() error         { return nil }
func (f *fakeGh) RepoExists(string) bool {
	f.existsSeen++
	if f.existsFlip && f.existsSeen == 1 {
		return false
	}
	return f.exists
}
func (f *fakeGh) RepoURL(string) (string, error)          { return f.url, nil }
func (f *fakeGh) LatestReleaseTag(string) (string, error) { return f.latestTag, nil }
func (f *fakeGh) RunMisonInstaller() error {
	f.installerRun = true
	return nil
}
func (f *fakeGh) CreatePrivateRepo(name string) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, name)
	return "https://github.com/me/" + name + ".git", nil
}

// fakeReport records one-way notifications.
type fakeReport struct{ calls []string }

func (f *fakeReport) Step(msg string)   { f.calls = append(f.calls, "step:"+msg) }
func (f *fakeReport) Synced(msg string) { f.calls = append(f.calls, "synced:"+msg) }
func (f *fakeReport) Warn(msg string)   { f.calls = append(f.calls, "warn:"+msg) }
func (f *fakeReport) Fail(msg string)   { f.calls = append(f.calls, "fail:"+msg) }
func (f *fakeReport) Line(msg string)   { f.calls = append(f.calls, "line:"+msg) }
func (f *fakeReport) ToolLine(mark, name, detail string) {
	f.calls = append(f.calls, "tool:"+mark+":"+name+":"+detail)
}

func (f *fakeReport) has(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// fakeAsk records blocking questions.
type fakeAsk struct {
	confirm   bool
	confirmQs []string
	conflicts []string
	tool      env.Tool
}

func (f *fakeAsk) Confirm(q string) bool {
	f.confirmQs = append(f.confirmQs, q)
	return f.confirm
}
func (f *fakeAsk) ResolveConflict(c env.Conflict) (env.Tool, error) {
	f.conflicts = append(f.conflicts, c.Name)
	return f.tool, nil
}

func newTestFlows(t *testing.T) (*Flows, *fakeMise, *bytes.Buffer) {
	return newTestFlowsWith(t, &fakeRepo{})
}

func newTestFlowsWith(t *testing.T, repo *fakeRepo) (*Flows, *fakeMise, *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	fm := &fakeMise{}
	out := &bytes.Buffer{}
	f := &Flows{
		Home: t.TempDir(),
		UI:   newBufferReporter(out),
		Ask:  &fakeAsk{},
		Mise: fm,
		Look: foundLookPath,
		Git:  func(string) EnvRepoIface { return repo },
		Gh:   &fakeGh{installed: true, authed: true},
	}
	fm.home = f.Home
	return f, fm, out
}

// bufferReporter adapts a bytes.Buffer to Reporter for output checks.
type bufferReporter struct{ inner *bytes.Buffer }

func newBufferReporter(b *bytes.Buffer) *bufferReporter { return &bufferReporter{b} }

func (r *bufferReporter) Step(msg string)   { r.inner.WriteString("✓ " + msg + "\n") }
func (r *bufferReporter) Synced(msg string) { r.inner.WriteString("↻ " + msg + "\n") }
func (r *bufferReporter) Warn(msg string)   { r.inner.WriteString("⚠ " + msg + "\n") }
func (r *bufferReporter) Fail(msg string)   { r.inner.WriteString("✗ " + msg + "\n") }
func (r *bufferReporter) Line(msg string)   { r.inner.WriteString(msg + "\n") }
func (r *bufferReporter) ToolLine(mark, name, detail string) {
	if detail == "" {
		r.inner.WriteString(mark + " " + name + "\n")
		return
	}
	r.inner.WriteString(mark + " " + name + " (" + detail + ")\n")
}

func readToml(t *testing.T, f *Flows) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.Home, ".mison", "env", "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	return string(data)
}

func TestRunInstallWritesDeclarationAndApplies(t *testing.T) {
	f, fm, out := newTestFlows(t)

	if err := f.RunInstall([]string{"node@22", "go"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	toml := readToml(t, f)
	if !strings.Contains(toml, `node = "22"`) {
		t.Errorf("mise.toml missing node:\n%s", toml)
	}
	if !strings.Contains(toml, `go = "latest"`) {
		t.Errorf("mise.toml missing go latest:\n%s", toml)
	}
	want := []string{"install node@22", "install go@latest", "lock --global"}
	if len(fm.execCalls) != len(want) {
		t.Fatalf("exec calls = %v, want %v", fm.execCalls, want)
	}
	for i := range want {
		if fm.execCalls[i] != want[i] {
			t.Errorf("exec calls = %v, want %v", fm.execCalls, want)
		}
	}
	if !strings.Contains(out.String(), "Installing node@22, go") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunInstallInvalidSpec(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if err := f.RunInstall([]string{"node@"}, PolicyAsk); err == nil {
		t.Fatal("RunInstall() expected error for node@")
	}
}

func TestRunInstallInstallsMiseWhenMissing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	f.Look = func(string) (string, error) { return "", errors.New("not found") }

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !fm.installDone {
		t.Error("RunInstaller() was not called")
	}
	if !strings.Contains(out.String(), "Installing mise") {
		t.Errorf("output = %q, want mise install step", out.String())
	}
}

func TestRunInstallCreatesSymlink(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	target, err := os.Readlink(filepath.Join(f.Home, ".config", "mise", "config.toml"))
	if err != nil {
		t.Fatalf("global config symlink missing: %v", err)
	}
	if target != filepath.Join(f.Home, ".mison", "env", "mise.toml") {
		t.Errorf("symlink target = %q", target)
	}
}

func TestRunUninstallRemovesDeclarationAndTool(t *testing.T) {
	f, fm, _ := newTestFlows(t)
	f.Ask = &fakeAsk{confirm: true}
	if err := f.RunInstall([]string{"node", "go"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.11.0", "go", "1.25.1")

	if err := f.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}

	toml := readToml(t, f)
	if strings.Contains(toml, "node") {
		t.Errorf("node still in mise.toml:\n%s", toml)
	}
	if !strings.Contains(toml, `go =`) {
		t.Errorf("go lost from mise.toml:\n%s", toml)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "uninstall --all node") {
		t.Errorf("exec calls = %v, want uninstall --all node", fm.execCalls)
	}
}

func TestRunUninstallAbortsWhenDeclined(t *testing.T) {
	f, _, _ := newTestFlows(t)
	f.Ask = &fakeAsk{confirm: false}
	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}

	if err := f.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	toml := readToml(t, f)
	if !strings.Contains(toml, "node") {
		t.Errorf("node should remain after declined prompt:\n%s", toml)
	}
}

func TestRunUninstallUnknownTool(t *testing.T) {
	f, _, _ := newTestFlows(t)
	f.Ask = &fakeAsk{confirm: true}
	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	if err := f.RunUninstall([]string{"python"}, false, PolicyAsk); err == nil {
		t.Fatal("RunUninstall() expected error for unknown tool")
	}
}

func TestRunStatusRendersStates(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22", "go@1.25", "python@3.13"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.11.0", "go", "1.24.0")

	if err := f.RunStatus(); err != nil {
		t.Fatalf("RunStatus() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"✓ node (22)",
		"⚠ go (declared 1.25, installed 1.24.0 — run mison sync)",
		"✗ python (missing",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunSyncAppliesMissing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.execCalls = nil

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "install") {
		t.Errorf("exec calls = %v, want install", fm.execCalls)
	}
	if !strings.Contains(out.String(), "Environment synchronized") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunSyncNoopWhenAligned(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.11.0")
	fm.execCalls = nil

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if len(fm.execCalls) != 0 {
		t.Errorf("exec calls = %v, want none", fm.execCalls)
	}
	if !strings.Contains(out.String(), "Already synchronized") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunSyncPruneRemovesOrphans(t *testing.T) {
	f, fm, _ := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.11.0", "ripgrep", "14.1.0")
	fm.execCalls = nil

	if err := f.RunSync(true, PolicyAsk); err != nil {
		t.Fatalf("RunSync(prune) error = %v", err)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "uninstall --all ripgrep") {
		t.Errorf("exec calls = %v, want orphan removal", fm.execCalls)
	}
}

func TestRunSyncWithoutEnvironment(t *testing.T) {
	f, _, _ := newTestFlows(t)
	err := f.RunSync(false, PolicyAsk)
	if err == nil || !strings.Contains(err.Error(), "mison init") {
		t.Fatalf("RunSync() error = %v, want init hint", err)
	}
}

func TestRunSyncRestoresGlobalSymlink(t *testing.T) {
	f, fm, _ := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	// simulate a machine that got the env dir by cloning: symlink removed
	if err := os.Remove(filepath.Join(f.Home, ".config", "mise", "config.toml")); err != nil {
		t.Fatal(err)
	}
	fm.execCalls = nil

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if target, err := os.Readlink(filepath.Join(f.Home, ".config", "mise", "config.toml")); err != nil || target == "" {
		t.Fatalf("global symlink not restored by sync: %v", err)
	}
}

func TestRunInstallWarnsStillMissing(t *testing.T) {
	f, _, out := newTestFlows(t)
	// fakeMise installs nothing, so node stays missing after Exec

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "node not visible to mise") {
		t.Errorf("missing post-install verification warning:\n%s", out.String())
	}
}

func TestRunSyncWarnsStillMissing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.execCalls = nil

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if !strings.Contains(out.String(), "node not visible to mise") {
		t.Errorf("missing post-sync verification warning:\n%s", out.String())
	}
}

func TestInstallPushesWhenRepoConnected(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, _, _ := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node@22"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "install: node" {
		t.Errorf("pushes = %v", repo.pushes)
	}
}

func TestInstallShowsRemoteMergeNotice(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"go"}}
	f, _, out := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "↻ Remote had new changes (go) — merged automatically") {
		t.Errorf("missing ↻ notice:\n%s", out.String())
	}
}

func TestInstallDeferredPushOnFailure(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pushErr: errors.New("network unreachable")}
	f, _, out := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() should not fail when push fails: %v", err)
	}
	if !strings.Contains(out.String(), "could not push") {
		t.Errorf("missing deferred-push warning:\n%s", out.String())
	}
}

func TestUninstallPushes(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, _ := newTestFlowsWith(t, repo)
	f.Ask = &fakeAsk{confirm: true}
	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.1.0")
	repo.pushes = nil

	if err := f.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "uninstall: node" {
		t.Errorf("pushes = %v", repo.pushes)
	}
}

func TestSyncPullsBeforeApply(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"node"}}
	f, fm, out := newTestFlowsWith(t, repo)
	if err := f.RunInstall([]string{"rg"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	repo.pushes = nil
	fm.execCalls = nil

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if repo.pulls != 1 {
		t.Errorf("pulls = %d, want 1", repo.pulls)
	}
	if !strings.Contains(out.String(), "↻ New changes: node") {
		t.Errorf("missing pull notice:\n%s", out.String())
	}
}

func TestRunInitCreatesRepoAndPushes(t *testing.T) {
	repo := &fakeRepo{}
	f, fm, out := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: false, authed: false}
	ghc := f.Gh.(*fakeGh)

	if err := f.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}

	if len(ghc.created) != 1 || ghc.created[0] != "mison-environment" {
		t.Errorf("created repos = %v", ghc.created)
	}
	if repo.remote == "" {
		t.Error("remote not set")
	}
	if len(repo.pushes) == 0 {
		t.Error("initial push missing")
	}

	tomlData, err := os.ReadFile(filepath.Join(f.Home, ".mison", "env", "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tomlData), "gh") {
		t.Errorf("gh must be declared in mise.toml:\n%s", tomlData)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "install gh@latest") {
		t.Errorf("gh install call missing: %v", fm.execCalls)
	}
	if !strings.Contains(out.String(), "Environment ready") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunInitConnectsExistingRepo(t *testing.T) {
	repo := &fakeRepo{isRepo: true, remote: "https://github.com/me/env.git"}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true}

	if err := f.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if repo.pulls != 1 {
		t.Errorf("pulls = %d, want 1", repo.pulls)
	}
}

func TestRunInitSecondMachineConnectsExistingRepo(t *testing.T) {
	repo := &fakeRepo{}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true, exists: true, url: "https://github.com/me/mison-environment.git"}

	if err := f.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	ghc := f.Gh.(*fakeGh)
	if len(ghc.created) != 0 {
		t.Errorf("must not create an existing repo: %v", ghc.created)
	}
	if len(repo.connected) != 1 || repo.connected[0] != "https://github.com/me/mison-environment.git" {
		t.Errorf("Connect calls = %v", repo.connected)
	}
}

func TestRunInitExistingEmptyRepoSeedsInitialPush(t *testing.T) {
	repo := &fakeRepo{remoteEmpty: true}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true, exists: true, url: "https://github.com/me/mison-environment.git"}

	if err := f.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if len(repo.connected) != 1 {
		t.Errorf("Connect calls = %v", repo.connected)
	}
	if len(repo.pushes) == 0 || repo.pushes[0] != "mison: init environment" {
		t.Errorf("seed push missing: %v", repo.pushes)
	}
}

func TestRunInitGhNotInstalledFlow(t *testing.T) {
	repo := &fakeRepo{}
	f, fm, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: false, authed: false}

	if err := f.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "install gh@latest") {
		t.Errorf("gh bootstrap missing: %v", fm.execCalls)
	}
	if !f.Gh.(*fakeGh).authed {
		t.Error("auth login not performed")
	}
}

// --- interaction-port contract tests (fake ports) ---

func wireFakes(f *Flows) (*fakeReport, *fakeAsk) {
	rep := &fakeReport{}
	ask := &fakeAsk{}
	f.UI = rep
	f.Ask = ask
	return rep, ask
}

func TestUninstallFlowAsksConfirmationBeforeRemoving(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, _ := newTestFlowsWith(t, repo)
	rep, ask := wireFakes(f)
	ask.confirm = true

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.1.0")
	repo.pushes = nil
	rep.calls = nil

	if err := f.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	if len(ask.confirmQs) != 1 || !strings.Contains(ask.confirmQs[0], "Remove node") {
		t.Errorf("questions = %v, want one 'Remove node...'", ask.confirmQs)
	}
	if !rep.has("step:Removing node") {
		t.Errorf("calls = %v, want removal step after confirmation", rep.calls)
	}
	if len(repo.pushes) != 1 {
		t.Errorf("pushes = %v, want auto-push after confirmed removal", repo.pushes)
	}
}

func TestUninstallFlowDeclinedStopsBeforeAnyChange(t *testing.T) {
	f, _, _ := newTestFlows(t)
	rep, ask := wireFakes(f)
	ask.confirm = false

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	rep.calls = nil

	if err := f.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	if len(ask.confirmQs) == 0 {
		t.Fatal("no confirmation asked")
	}
	if rep.has("step:Removing") {
		t.Errorf("declined confirmation must stop the flow, calls = %v", rep.calls)
	}
}

func TestSyncFlowNotifiesRemoteMergeViaSyncedPort(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"node"}}
	f, _, _ := newTestFlowsWith(t, repo)
	rep, _ := wireFakes(f)

	if err := f.RunInstall([]string{"rg"}, PolicyAsk); err != nil {
		t.Fatal(err)
	}
	rep.calls = nil

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if !rep.has("synced:New changes: node") {
		t.Errorf("remote merge must surface through Reporter.Synced, calls = %v", rep.calls)
	}
}

func TestConflictResolutionRoutesThroughPrompter(t *testing.T) {
	repo := &fakeRepo{isRepo: true, conflict: &env.Conflict{
		Name: "node", Base: tool("node", "20"),
		Local: tool("node", "24"), Remote: tool("node", "22"),
	}}
	f, _, _ := newTestFlowsWith(t, repo)
	_, ask := wireFakes(f)
	ask.tool = tool("node", "24")

	if err := f.RunInstall([]string{"rg"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if len(ask.conflicts) != 1 || ask.conflicts[0] != "node" {
		t.Errorf("conflicts = %v, want [node] routed through Prompter", ask.conflicts)
	}
	if len(repo.pushes) != 1 {
		t.Errorf("pushes = %v, want push after resolution", repo.pushes)
	}
}

func TestRunInitPersistsRepoNameLocally(t *testing.T) {
	repo := &fakeRepo{}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: false}

	if err := f.RunInit("my-env"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(f.Home, ".mison", "config.toml"))
	if err != nil {
		t.Fatalf("local config missing: %v", err)
	}
	if !strings.Contains(string(data), `repo = "my-env"`) {
		t.Fatalf("repo name not persisted:\n%s", data)
	}

	// a second machine (fresh local state) must prefer the persisted
	// name over any default drift: simulate by resetting the fake repo
	repo2 := &fakeRepo{}
	f.Git = func(string) EnvRepoIface { return repo2 }
	f.Gh = &fakeGh{installed: true, authed: true, exists: true, url: "https://github.com/me/my-env.git"}
	if err := f.RunInit("ignored-default"); err != nil {
		t.Fatalf("second RunInit() error = %v", err)
	}
	if len(repo2.connected) != 1 || repo2.connected[0] != "https://github.com/me/my-env.git" {
		t.Errorf("persisted repo not used: %v", repo2.connected)
	}
}

func TestRunInitDoesNotWriteReadme(t *testing.T) {
	repo := &fakeRepo{}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true}

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.Home, ".mison", "env", "README.md")); !os.IsNotExist(err) {
		t.Errorf("README.md must not be created: %v", err)
	}
}

func TestInstallFutureSchemaPushIsFatal(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pushErr: fmt.Errorf("parse origin/main:mise.toml: %w", env.ErrFutureSchema)}
	f, _, out := newTestFlowsWith(t, repo)

	err := f.RunInstall([]string{"node"}, PolicyAsk)
	if err == nil {
		t.Fatal("RunInstall must propagate future-schema push failure")
	}
	if !strings.Contains(out.String(), "✗") {
		t.Fatal("must report via Fail port (✗)")
	}
	if strings.Contains(out.String(), "will retry on next sync") {
		t.Fatal("future-schema must not be deferred to next sync")
	}
}

func TestSyncFutureSchemaPullIsFatal(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pullErr: fmt.Errorf("parse origin/main:mise.toml: %w", env.ErrFutureSchema)}
	f, _, _ := newTestFlowsWith(t, repo)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}

	if err := f.RunSync(false, PolicyAsk); err == nil {
		t.Fatal("RunSync must abort on future-schema remote")
	} else if !errors.Is(err, env.ErrFutureSchema) {
		t.Fatalf("error must wrap ErrFutureSchema, got: %v", err)
	}
}

func TestRunInitExplicitFlagWinsOverPersisted(t *testing.T) {
	repo := &fakeRepo{}
	f, _, _ := newTestFlowsWith(t, repo)

	// a different name persisted from a previous init
	cfgDir := filepath.Join(f.Home, ".mison")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("# managed by mison\nrepo = \"other-env\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.RunInit("mison-env"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if len(repo.connected) > 0 || repo.remote != "https://github.com/me/mison-env.git" {
		t.Fatalf("explicit flag must win, connected=%v remote=%v", repo.connected, repo.remote)
	}
	persisted, err := os.ReadFile(filepath.Join(cfgDir, "config.toml"))
	if err != nil || !strings.Contains(string(persisted), "mison-env") {
		t.Fatalf("flag choice must be persisted: %v %q", err, persisted)
	}
}

func TestVerifyWarnsWhenListFails(t *testing.T) {
	f, fm, out := newTestFlows(t)
	fm.listErr = errors.New("mise ls broke")

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "could not verify installation") {
		t.Fatalf("list failure must warn, output:\n%s", out.String())
	}
}

func TestSyncPruneContinuesPastFailures(t *testing.T) {
	f, fm, _ := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	// two orphans: jq sorts first and fails; node must still be attempted
	fm.setInstalled("node", "22", "jq", "1.7")
	fm.execFailArg = "jq"

	err := f.RunSync(true, PolicyAsk) // --prune: unattended
	if err == nil || !strings.Contains(err.Error(), "jq") {
		t.Fatalf("RunSync() must report the failed prune, got: %v", err)
	}
	// node must still have been pruned after jq failed
	attempted := false
	for _, c := range fm.execCalls {
		if strings.Contains(c, "node") {
			attempted = true
		}
	}
	if !attempted {
		t.Fatalf("prune must continue past failures, execCalls: %v", fm.execCalls)
	}
}

func TestRunInitCreateRaceFallsBackToConnect(t *testing.T) {
	repo := &fakeRepo{remoteEmpty: false}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{
		installed: true, authed: true,
		exists: true, existsFlip: true, url: "https://github.com/me/mison-env.git",
		createErr: errors.New("name already exists on this account"),
	}

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() must fall back to connect on create race, got: %v", err)
	}
	if len(repo.connected) != 1 || repo.connected[0] != "https://github.com/me/mison-env.git" {
		t.Fatalf("must connect to the raced repo, connected: %v", repo.connected)
	}
}

func TestInstallRefreshesLockBeforePush(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, _ := newTestFlowsWith(t, repo)
	fm.lockResult = "# lock v1\n"

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	// lock regen must run after install and land in the same push
	lockIdx, installIdx := -1, -1
	for i, c := range fm.execCalls {
		if strings.HasPrefix(c, "install ") {
			installIdx = i
		}
		if c == "lock --global" {
			lockIdx = i
		}
	}
	if installIdx == -1 || lockIdx == -1 || lockIdx < installIdx {
		t.Fatalf("lock must be refreshed after apply, execCalls: %v", fm.execCalls)
	}
	if len(repo.pushes) == 0 {
		t.Fatal("declaration push must still happen")
	}
	data, err := os.ReadFile(filepath.Join(f.Home, ".mison", "env", "mise.lock"))
	if err != nil || string(data) != "# lock v1\n" {
		t.Fatalf("lockfile not written through symlink: %v %q", err, data)
	}
}

func TestInstallLockFailureWarnsAndDefers(t *testing.T) {
	f, fm, out := newTestFlows(t)
	fm.execFailArg = "lock"

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() must survive lock failure, got: %v", err)
	}
	if !strings.Contains(out.String(), "could not refresh lockfile") {
		t.Fatalf("lock failure must warn, output:\n%s", out.String())
	}
}

func TestSyncPushesRefreshedLock(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, out := newTestFlowsWith(t, repo)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	// a declared-but-missing tool forces the apply path
	if err := os.WriteFile(f.layout().MiseToml, []byte("[tools]\nnode = \"22\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm.lockResult = "# lock v2\n"

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	found := false
	for _, p := range repo.pushes {
		if p == "mison: refresh lock" {
			found = true
		}
	}
	if !found {
		t.Fatalf("changed lock must be pushed, pushes: %v", repo.pushes)
	}
	if !strings.Contains(out.String(), "Refreshing lockfile") {
		t.Fatalf("lock refresh must be visible, output:\n%s", out.String())
	}
}

func TestSyncSkipsLockPushWhenUnchanged(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, _, _ := newTestFlowsWith(t, repo)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	// lockResult empty: mise "regenerates" nothing — lock stays absent
	// before and after → unchanged → no lock push

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	for _, p := range repo.pushes {
		if p == "mison: refresh lock" {
			t.Fatalf("unchanged lock must not be pushed, pushes: %v", repo.pushes)
		}
	}
}

func TestRunInitRefreshesLock(t *testing.T) {
	repo := &fakeRepo{}
	f, fm, _ := newTestFlowsWith(t, repo)
	fm.lockResult = "# lock v1\n"

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	found := false
	for _, p := range repo.pushes {
		if p == "mison: refresh lock" {
			found = true
		}
	}
	if !found {
		t.Fatalf("changed lock must be pushed after init install, pushes: %v", repo.pushes)
	}
}

func TestSyncNeverPrunesGh(t *testing.T) {
	f, fm, _ := newTestFlows(t)
	ask := &fakeAsk{confirm: true}
	f.Ask = ask
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	// gh installed via the bootstrap chain but not declared
	fm.setInstalled("gh", "2.62.0")

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if len(ask.confirmQs) != 0 {
		t.Fatalf("gh must never be offered as an orphan: %v", ask.confirmQs)
	}
	for _, c := range fm.execCalls {
		if strings.Contains(c, "gh") {
			t.Fatalf("gh must not be pruned, execCalls: %v", fm.execCalls)
		}
	}
}

func TestPushRegeneratesLockAfterRemoteMerge(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"node"}}
	f, fm, _ := newTestFlowsWith(t, repo)
	// first regen (pre-push) writes v1; the merge resets the worktree to
	// the remote's lock; the second regen writes v2 — content differs,
	// so the refresh must be pushed
	fm.lockResults = []string{"# lock v1", "# lock v2"}

	if err := f.RunInstall([]string{"go"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	wantPushes := []string{"install: go", "mison: refresh lock"}
	if len(repo.pushes) != len(wantPushes) {
		t.Fatalf("pushes = %v, want %v", repo.pushes, wantPushes)
	}
	for i := range wantPushes {
		if repo.pushes[i] != wantPushes[i] {
			t.Fatalf("pushes = %v, want %v", repo.pushes, wantPushes)
		}
	}
}

func TestPushWithoutMergeSkipsLockRepush(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, _ := newTestFlowsWith(t, repo)
	fm.lockResult = "# lock v1\n"

	if err := f.RunInstall([]string{"go"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "install: go" {
		t.Fatalf("pushes = %v, want only [install: go]", repo.pushes)
	}
}

func TestRunInitRebindsToExplicitRepo(t *testing.T) {
	repo := &fakeRepo{isRepo: true, remote: "https://github.com/me/old-env.git"}
	f, _, _ := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true, url: "https://github.com/me/new-env.git"}

	if err := f.RunInit("new-env"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if len(repo.setURLs) != 1 || repo.setURLs[0] != "https://github.com/me/new-env.git" {
		t.Fatalf("explicit flag must re-bind the remote: %v", repo.setURLs)
	}
	if repo.pulls != 1 {
		t.Fatalf("re-bind must be followed by a pull, pulls = %d", repo.pulls)
	}
}

func TestRunInitNoRebindWithoutFlag(t *testing.T) {
	repo := &fakeRepo{isRepo: true, remote: "https://github.com/me/old-env.git"}
	f, _, _ := newTestFlowsWith(t, repo)

	if err := f.RunInit(""); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if len(repo.setURLs) != 0 {
		t.Fatalf("no flag → no re-bind, setURLs: %v", repo.setURLs)
	}
	if repo.pulls != 1 {
		t.Fatalf("existing remote must still pull, pulls = %d", repo.pulls)
	}
}

func TestInstallFailureDeclaresNothing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	fm.execErr = errors.New("no version found for tool bogus-tool")

	err := f.RunInstall([]string{"bogus-tool"}, PolicyAsk)
	if err == nil {
		t.Fatal("RunInstall must fail when mise install fails")
	}
	if !strings.Contains(err.Error(), "nothing declared") {
		t.Fatalf("error must state that nothing was declared: %v", err)
	}
	toml := readToml(t, f)
	if strings.Contains(toml, "bogus-tool") {
		t.Fatalf("failed tool must not be declared:\n%s", toml)
	}
	if !strings.Contains(out.String(), "✗ bogus-tool") {
		t.Fatalf("failure must render as an outcome:\n%s", out.String())
	}
}

func TestSyncApplyFailureHintsRemoval(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.layout().MiseToml, []byte("[tools]\nbogus = \"9.9\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm.execErr = errors.New("mise install: no version found")

	err := f.RunSync(false, PolicyAsk)
	if err != nil {
		t.Fatalf("sync must survive per-tool apply failures, got: %v", err)
	}
	if !strings.Contains(out.String(), "failed to apply") || !strings.Contains(out.String(), "mison uninstall") {
		t.Fatalf("apply failure must warn with removal hint:\n%s", out.String())
	}
}

func TestStatusWithoutEnvironmentHintsInit(t *testing.T) {
	f, _, _ := newTestFlows(t) // fresh home, no init
	err := f.RunStatus()
	if err == nil || !strings.Contains(err.Error(), "mison init") {
		t.Fatalf("status before init must hint init, got: %v", err)
	}
}

func TestStatusMismatchHintsSync(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.layout().MiseToml, []byte("[tools]\nnode = \"22\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "20")

	if err := f.RunStatus(); err != nil {
		t.Fatalf("RunStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "run mison sync") || !strings.Contains(out.String(), "declared 22, installed 20") {
		t.Fatalf("mismatch must show versions and sync hint:\n%s", out.String())
	}
}

func TestPushFailureHintsRebindWhenRepoGone(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pushErr: errors.New("git push: Repository not found")}
	f, _, out := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() must stay warn-and-defer, got: %v", err)
	}
	if !strings.Contains(out.String(), "re-bind") {
		t.Fatalf("repository-not-found must hint re-binding:\n%s", out.String())
	}
}

func TestSyncOrphanDeclinedKeepsTools(t *testing.T) {
	f, fm, out := newTestFlows(t)
	f.Ask = &fakeAsk{confirm: false}
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22") // node installed but undeclared

	if err := f.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if !strings.Contains(out.String(), "kept") {
		t.Fatalf("declined prune must be reported as kept:\n%s", out.String())
	}
	for _, c := range fm.execCalls {
		if strings.Contains(c, "uninstall") {
			t.Fatalf("declined prune must not uninstall, execCalls: %v", fm.execCalls)
		}
	}
}

func TestConcurrentRunGuarded(t *testing.T) {
	f, _, out := newTestFlows(t)

	// simulate a second terminal already running mison
	g, err := lockfile.Acquire(f.layout().RunLock)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err == nil {
		t.Fatal("RunInstall must refuse while another run holds the lock")
	} else if !strings.Contains(err.Error(), "another mison") {
		t.Fatalf("error must explain the lock, got: %v", err)
	}
	if !strings.Contains(out.String(), "✗") {
		t.Fatalf("must report via Fail port:\n%s", out.String())
	}
}

func TestPushFailureHintsReloginOnAuthError(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pushErr: errors.New("git push: could not read Username for 'https://github.com': terminal prompts disabled")}
	f, _, out := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() must stay warn-and-defer, got: %v", err)
	}
	if !strings.Contains(out.String(), "gh auth login") {
		t.Fatalf("auth failure must hint re-login:\n%s", out.String())
	}
}

func TestStatusCountsProjectShadowedTools(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	// go declared in the mison declaration but ACTIVATED by a project
	// config that shadows the same name (real-world mise reports the
	// project as the single source)
	fm.extra = []miserepo.Entry{{
		Name: "go", Version: "1.26.8", Active: true,
		Source: "/some/project/mise.toml",
	}}
	if err := os.WriteFile(f.layout().MiseToml, []byte("[tools]\ngo = \"1.26\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.RunStatus(); err != nil {
		t.Fatalf("RunStatus() error = %v", err)
	}
	if strings.Contains(out.String(), "✗ go") {
		t.Fatalf("project-shadowed declared tool must count as installed:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "✓ go") {
		t.Fatalf("go should be reported OK:\n%s", out.String())
	}
}

func TestRunInitShowsGitHubAccount(t *testing.T) {
	repo := &fakeRepo{}
	f, _, out := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true, whoami: "dev-hann"}

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if !strings.Contains(out.String(), "GitHub account: dev-hann") {
		t.Fatalf("init must show the active account before creating anything:\n%s", out.String())
	}
	// the notice must precede repo creation (create path ran last)
	if !strings.Contains(out.String(), "Creating environment repository") {
		t.Fatalf("expected create path, output:\n%s", out.String())
	}
}

func TestStatusWarnsPathBackedTools(t *testing.T) {
	f, _, out := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	toml := "[tools]\njq = \"latest\"\n\n[tools.mytool]\nversion = \"1.0\"\npath = \"/Users/me/dev/mytool\"\n"
	if err := os.WriteFile(f.layout().MiseToml, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	fm, _ := f.Mise.(*fakeMise)
	fm.setInstalled("jq", "1.8")

	if err := f.RunStatus(); err != nil {
		t.Fatalf("RunStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "path-backed tools are machine-local") ||
		!strings.Contains(out.String(), "mytool (/Users/me/dev/mytool)") {
		t.Fatalf("path-backed declaration must warn:\n%s", out.String())
	}
}

func TestStatusNoWarningWithoutPathTools(t *testing.T) {
	f, _, out := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	fm, _ := f.Mise.(*fakeMise)
	fm.setInstalled("jq", "1.8")

	if err := f.RunStatus(); err != nil {
		t.Fatalf("RunStatus() error = %v", err)
	}
	if strings.Contains(out.String(), "path-backed") {
		t.Fatalf("registry-only declarations must not warn:\n%s", out.String())
	}
}

func TestInitActivatesShell(t *testing.T) {
	f, _, out := newTestFlows(t)
	f.Shell = "/bin/zsh"

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	rc := filepath.Join(f.Home, ".zshrc")
	data, err := os.ReadFile(rc)
	if err != nil || !strings.Contains(string(data), "mise activate zsh") {
		t.Fatalf("rc must gain the activation block: %v %q", err, data)
	}
	if !strings.Contains(out.String(), "Added mise activation") {
		t.Fatalf("output must report the addition:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "exec zsh") {
		t.Fatalf("epilogue must offer exec zsh:\n%s", out.String())
	}
}

func TestInitShellActivationIdempotent(t *testing.T) {
	f, _, _ := newTestFlows(t)
	f.Shell = "/bin/zsh"
	for i := 0; i < 2; i++ {
		if err := f.RunInit(DefaultRepoName); err != nil {
			t.Fatalf("RunInit() #%d error = %v", i, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(f.Home, ".zshrc"))
	if got := strings.Count(string(data), "mise activate zsh"); got != 1 {
		t.Fatalf("activation block must appear once, got %d:\n%s", got, data)
	}
}

func TestInitSkipsActivationWhenAlreadyWired(t *testing.T) {
	f, _, out := newTestFlows(t)
	f.Shell = "/bin/zsh"
	rc := filepath.Join(f.Home, ".zshrc")
	if err := os.WriteFile(rc, []byte("eval \"$(mise activate zsh)\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	data, _ := os.ReadFile(rc)
	if len(data) != len("eval \"$(mise activate zsh)\"\n") {
		t.Fatalf("already-activated rc must stay untouched:\n%s", data)
	}
	if strings.Contains(out.String(), "Added mise activation") {
		t.Fatalf("no addition report expected:\n%s", out.String())
	}
}

func TestInitUnknownShellPrintsHintOnly(t *testing.T) {
	f, _, out := newTestFlows(t)
	f.Shell = "/usr/bin/fish"

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if !strings.Contains(out.String(), "Add mise to your shell") {
		t.Fatalf("unknown shell must get a manual hint:\n%s", out.String())
	}
	if entries, _ := filepath.Glob(filepath.Join(f.Home, ".*rc*")); len(entries) > 0 {
		t.Fatalf("unknown shell must not write rc files: %v", entries)
	}
}

func TestInitNoShellSetupSkips(t *testing.T) {
	f, _, out := newTestFlows(t)
	f.Shell = "/bin/zsh"
	f.NoShellSetup = true

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.Home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal("--no-shell-setup must not touch rc files")
	}
	if strings.Contains(out.String(), "Add mise to your shell") {
		t.Fatalf("opt-out must stay silent about shell setup:\n%s", out.String())
	}
}

func TestRunInitStoresAccount(t *testing.T) {
	f, _, _ := newTestFlows(t)
	f.Account = "dev-hann"
	f.Gh = &fakeGh{installed: true, authed: true, whoami: "dev-hann"}

	if err := f.RunInit(DefaultRepoName); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	data, err := os.ReadFile(f.repoConfigPath())
	if err != nil || !strings.Contains(string(data), `account = "dev-hann"`) {
		t.Fatalf("account must persist to config.toml: %v %q", err, data)
	}
}

func TestPushRefusesOnAccountMismatch(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, _, out := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true, whoami: "hann-yun"}
	if err := os.MkdirAll(filepath.Dir(f.repoConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.repoConfigPath(), []byte("repo = \"mison-env\"\naccount = \"dev-hann\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := f.RunInstall([]string{"node"}, PolicyAsk)
	if err == nil || !strings.Contains(err.Error(), "gh auth switch") {
		t.Fatalf("mismatched account must refuse the push with switch hint: %v", err)
	}
	if !strings.Contains(out.String(), "✗") {
		t.Fatalf("must report via Fail port:\n%s", out.String())
	}
	for _, p := range repo.pushes {
		if strings.Contains(p, "install") {
			t.Fatal("no push may happen under the wrong account")
		}
	}
}

func TestPushContinuesWhenAccountUnverifiable(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, _, out := newTestFlowsWith(t, repo)
	f.Gh = &fakeGh{installed: true, authed: true, whoamiErr: errors.New("offline")}
	if err := os.MkdirAll(filepath.Dir(f.repoConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.repoConfigPath(), []byte("account = \"dev-hann\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := f.RunInstall([]string{"node"}, PolicyAsk); err != nil {
		t.Fatalf("offline account check must degrade to a warning: %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "install: node" {
		t.Fatalf("push must proceed: %v", repo.pushes)
	}
	if !strings.Contains(out.String(), "could not verify GitHub account") {
		t.Fatalf("degradation must warn:\n%s", out.String())
	}
}

func TestSyncPullGatedByAccount(t *testing.T) {
	f, _, _ := newTestFlows(t)
	ask := &fakeAsk{}
	f.Ask = ask
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	f.Gh = &fakeGh{installed: true, authed: true, whoami: "hann-yun"}
	if err := os.MkdirAll(filepath.Dir(f.repoConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.repoConfigPath(), []byte("account = \"dev-hann\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{isRepo: true, pullErr: nil}
	f.Git = func(string) EnvRepoIface { return repo }

	if err := f.RunSync(false, PolicyAsk); err == nil || !strings.Contains(err.Error(), "gh auth switch") {
		t.Fatalf("sync pull must refuse under wrong account: %v", err)
	}
	if repo.pulls != 0 {
		t.Fatalf("no pull may happen: %d", repo.pulls)
	}
}

func TestRunUpdateDryRunShowsCandidatesOnly(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, out := newTestFlowsWith(t, repo)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	fm.bumpCandidates = []miserepo.BumpCandidate{
		{Name: "node", OldVersions: []string{"22.23.2"}, NewVersions: []string{"22.24.0"}},
	}

	if err := f.RunUpdate(nil, true, PolicyAsk); err != nil {
		t.Fatalf("RunUpdate(dry-run) error = %v", err)
	}
	if !strings.Contains(out.String(), "node 22.23.2 → 22.24.0") {
		t.Fatalf("dry run must list candidates:\n%s", out.String())
	}
	if len(repo.pushes) != 0 || len(fm.execCalls) != 0 {
		t.Fatalf("dry run must not apply anything: pushes=%v exec=%v", repo.pushes, fm.execCalls)
	}
}

func TestRunUpdateUpToDateNoop(t *testing.T) {
	f, _, out := newTestFlows(t)
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}

	if err := f.RunUpdate(nil, false, PolicyAsk); err != nil {
		t.Fatalf("RunUpdate() error = %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("no candidates → up-to-date notice:\n%s", out.String())
	}
}

func TestRunUpdateAppliesAfterConfirm(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, out := newTestFlowsWith(t, repo)
	f.Ask = &fakeAsk{confirm: true}
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	fm.bumpCandidates = []miserepo.BumpCandidate{
		{Name: "node", OldVersions: []string{"22.23.2"}, NewVersions: []string{"22.24.0"}},
	}
	fm.lockResult = "# lock bumped\n"

	if err := f.RunUpdate([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUpdate() error = %v", err)
	}
	var bumped bool
	for _, c := range fm.execCalls {
		if c == "lock --global --bump node" {
			bumped = true
		}
	}
	if !bumped || !containsStr(fm.execCalls, "install") {
		t.Fatalf("update must bump then install: %v", fm.execCalls)
	}
	if len(repo.pushes) != 1 || !strings.Contains(repo.pushes[0], "update:") {
		t.Fatalf("pushes = %v, want one update commit", repo.pushes)
	}
	if !strings.Contains(repo.pushes[0], "22.23.2") {
		t.Fatalf("commit message must carry versions: %v", repo.pushes)
	}
	_ = out
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestRunUpdateDeclinedAborts(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	f, fm, _ := newTestFlowsWith(t, repo)
	f.Ask = &fakeAsk{confirm: false}
	if _, err := f.layout().Ensure(); err != nil {
		t.Fatal(err)
	}
	fm.bumpCandidates = []miserepo.BumpCandidate{
		{Name: "node", OldVersions: []string{"22.23.2"}, NewVersions: []string{"22.24.0"}},
	}

	if err := f.RunUpdate(nil, false, PolicyAsk); err != nil {
		t.Fatalf("RunUpdate() decline must not error: %v", err)
	}
	for _, c := range fm.execCalls {
		if strings.Contains(c, "--bump") && !strings.Contains(c, "dry-run") {
			t.Fatalf("declined update must not bump: %v", fm.execCalls)
		}
	}
	if len(repo.pushes) != 0 {
		t.Fatalf("declined update must not push: %v", repo.pushes)
	}
}

func TestRunUpgradeNoopWhenCurrent(t *testing.T) {
	f, _, out := newTestFlows(t)
	f.Gh = &fakeGh{latestTag: "v9.9.9"}

	if err := f.RunUpgrade("v9.9.9"); err != nil {
		t.Fatalf("RunUpgrade() error = %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("same version → up-to-date notice:\n%s", out.String())
	}
}

func TestRunUpgradeRunsInstaller(t *testing.T) {
	f, _, out := newTestFlows(t)
	gh := &fakeGh{latestTag: "v0.5.0"}
	f.Gh = gh

	if err := f.RunUpgrade("v0.4.1"); err != nil {
		t.Fatalf("RunUpgrade() error = %v", err)
	}
	if !gh.installerRun {
		t.Fatal("installer must run for a newer release")
	}
	if !strings.Contains(out.String(), "v0.4.1") || !strings.Contains(out.String(), "v0.5.0") {
		t.Fatalf("upgrade must report versions:\n%s", out.String())
	}
}

func TestRunUpgradeRefusesDevBuild(t *testing.T) {
	f, _, _ := newTestFlows(t)
	gh := &fakeGh{latestTag: "v9.9.9"}
	f.Gh = gh

	if err := f.RunUpgrade("dev"); err == nil {
		t.Fatal("dev builds must refuse release comparison")
	}
	if gh.installerRun {
		t.Fatal("no installer run for dev builds")
	}
}

func TestRunUpgradeVPrefixNormalizes(t *testing.T) {
	f, _, out := newTestFlows(t)
	f.Gh = &fakeGh{latestTag: "v0.5.0"}

	if err := f.RunUpgrade("0.5.0"); err != nil {
		t.Fatalf("RunUpgrade() error = %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Fatalf("v-prefixed latest must match bare current:\\n%s", out.String())
	}
}
