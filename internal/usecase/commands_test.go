package usecase

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/repo/miserepo"
)

// fakeMise implements MiseRepoIface; entries default to mison-owned
// sources so OwnedTools passes them through.
type fakeMise struct {
	home        string
	extra       []miserepo.Entry // foreign/inactive extras
	execCalls   []string
	installDone bool
	execErr     error
	listErr     error
	execFailArg string
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
	f.execCalls = append(f.execCalls, joined)
	return nil
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
	installed bool
	authed    bool
	created   []string
	exists    bool
	url       string
}

func (f *fakeGh) IsInstalled() bool              { return f.installed }
func (f *fakeGh) AuthStatus() bool               { return f.authed }
func (f *fakeGh) AuthLogin() error               { f.authed = true; return nil }
func (f *fakeGh) SetupGit() error                { return nil }
func (f *fakeGh) RepoExists(string) bool         { return f.exists }
func (f *fakeGh) RepoURL(string) (string, error) { return f.url, nil }
func (f *fakeGh) CreatePrivateRepo(name string) (string, error) {
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

	if err := f.RunInstall([]string{"node@22", "go"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	toml := readToml(t, f)
	if !strings.Contains(toml, `node = "22"`) {
		t.Errorf("mise.toml missing node:\n%s", toml)
	}
	if !strings.Contains(toml, `go = "latest"`) {
		t.Errorf("mise.toml missing go latest:\n%s", toml)
	}
	if len(fm.execCalls) != 1 || fm.execCalls[0] != "install" {
		t.Errorf("exec calls = %v, want [install]", fm.execCalls)
	}
	if !strings.Contains(out.String(), "Installing node, go") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunInstallWithOSSpec(t *testing.T) {
	f, _, _ := newTestFlows(t)

	if err := f.RunInstall([]string{"docker"}, "linux", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	toml := readToml(t, f)
	if !strings.Contains(toml, `os = ["linux"]`) {
		t.Errorf("mise.toml missing os restriction:\n%s", toml)
	}
}

func TestRunInstallInvalidSpec(t *testing.T) {
	f, _, _ := newTestFlows(t)
	if err := f.RunInstall([]string{"node@"}, "", PolicyAsk); err == nil {
		t.Fatal("RunInstall() expected error for node@")
	}
}

func TestRunInstallInstallsMiseWhenMissing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	f.Look = func(string) (string, error) { return "", errors.New("not found") }

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node", "go"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	if err := f.RunUninstall([]string{"python"}, false, PolicyAsk); err == nil {
		t.Fatal("RunUninstall() expected error for unknown tool")
	}
}

func TestRunStatusRendersStates(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22", "go@1.25", "python@3.13"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.setInstalled("node", "22.11.0", "go", "1.24.0")

	if err := f.RunStatus(); err != nil {
		t.Fatalf("RunStatus() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"✓ node (22)",
		"⚠ go (declared 1.25, installed 1.24.0)",
		"✗ python (missing",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunSyncAppliesMissing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
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

func TestRunInstallWarnsOSSkip(t *testing.T) {
	f, _, out := newTestFlows(t)
	opposite := "linux"
	if runtime.GOOS == "linux" {
		opposite = "macos"
	}

	if err := f.RunInstall([]string{"docker"}, opposite, PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "docker: restricted to "+opposite+" — skipped on this machine") {
		t.Errorf("missing OS skip warning:\n%s", out.String())
	}
}

func TestRunInstallWarnsStillMissing(t *testing.T) {
	f, _, out := newTestFlows(t)
	// fakeMise installs nothing, so node stays missing after Exec

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "node not visible to mise") {
		t.Errorf("missing post-install verification warning:\n%s", out.String())
	}
}

func TestRunSyncWarnsStillMissing(t *testing.T) {
	f, fm, out := newTestFlows(t)
	if err := f.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
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

	if err := f.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "install: node" {
		t.Errorf("pushes = %v", repo.pushes)
	}
}

func TestInstallShowsRemoteMergeNotice(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"go"}}
	f, _, out := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "↻ Remote had new changes (go) — merged automatically") {
		t.Errorf("missing ↻ notice:\n%s", out.String())
	}
}

func TestInstallDeferredPushOnFailure(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pushErr: errors.New("network unreachable")}
	f, _, out := newTestFlowsWith(t, repo)

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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
	if err := f.RunInstall([]string{"rg"}, "", PolicyAsk); err != nil {
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

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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

	if err := f.RunInstall([]string{"rg"}, "", PolicyAsk); err != nil {
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

	if err := f.RunInstall([]string{"rg"}, "", PolicyAsk); err != nil {
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

	err := f.RunInstall([]string{"node"}, "", PolicyAsk)
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

	if err := f.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
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
