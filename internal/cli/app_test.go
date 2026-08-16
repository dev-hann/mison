package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/gitclient"
	"github.com/dev-hann/mison/internal/mise"
)

type fakeMise struct {
	installed   []mise.Tool
	execCalls   []string
	installDone bool
	execErr     error
}

func (f *fakeMise) IsInstalled() bool { return true }
func (f *fakeMise) Version() (string, error) {
	return "2026.1.1", nil
}
func (f *fakeMise) Install() error {
	f.installDone = true
	return nil
}
func (f *fakeMise) Exec(args ...string) error {
	if f.execErr != nil {
		return f.execErr
	}
	f.execCalls = append(f.execCalls, strings.Join(args, " "))
	return nil
}
func (f *fakeMise) InstalledTools() ([]mise.Tool, error) {
	return f.installed, nil
}

func foundLookPath(string) (string, error) { return "/usr/bin/mise", nil }

type fakeRepo struct {
	isRepo      bool
	remote      string
	pushes      []string
	pulls       int
	pushErr     error
	mergedOn    []string
	connected   []string
	remoteEmpty bool
	syncState   gitclient.SyncState
	remoteAdded []string
	syncErr     error
}

func (f *fakeRepo) IsRepo() bool { return f.isRepo }
func (f *fakeRepo) Init() error  { f.isRepo = true; return nil }
func (f *fakeRepo) RemoteAdd(url string) error {
	f.remote = url
	return nil
}
func (f *fakeRepo) RemoteURL() string { return f.remote }
func (f *fakeRepo) SmartPush(message string, _ gitclient.Resolver) ([]string, error) {
	if f.pushErr != nil {
		return nil, f.pushErr
	}
	f.pushes = append(f.pushes, message)
	return f.mergedOn, nil
}
func (f *fakeRepo) SmartPull(_ gitclient.Resolver) ([]string, error) {
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
func (f *fakeRepo) SyncStatus() (gitclient.SyncInfo, error) {
	return gitclient.SyncInfo{State: f.syncState, RemoteAdded: f.remoteAdded}, f.syncErr
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

func newTestApp(t *testing.T) (*App, *fakeMise, *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	return newTestAppWith(t, &fakeRepo{})
}

func newTestAppWith(t *testing.T, repo *fakeRepo) (*App, *fakeMise, *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	fm := &fakeMise{}
	out := &bytes.Buffer{}
	app := &App{
		Home:     t.TempDir(),
		Stdout:   out,
		In:       strings.NewReader(""),
		Mise:     fm,
		LookPath: foundLookPath,
		Git:      func(string) Repo { return repo },
		Gh:       &fakeGh{installed: true, authed: true},
	}
	return app, fm, out
}

func readToml(t *testing.T, app *App) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(app.Home, ".mison", "env", "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	return string(data)
}

func TestRunInstallWritesDeclarationAndApplies(t *testing.T) {
	app, fm, out := newTestApp(t)

	if err := app.RunInstall([]string{"node@22", "go"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	toml := readToml(t, app)
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
	app, _, _ := newTestApp(t)

	if err := app.RunInstall([]string{"docker"}, "linux", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	toml := readToml(t, app)
	if !strings.Contains(toml, `os = ["linux"]`) {
		t.Errorf("mise.toml missing os restriction:\n%s", toml)
	}
}

func TestRunInstallInvalidSpec(t *testing.T) {
	app, _, _ := newTestApp(t)
	if err := app.RunInstall([]string{"node@"}, "", PolicyAsk); err == nil {
		t.Fatal("RunInstall() expected error for node@")
	}
}

func TestRunInstallInstallsMiseWhenMissing(t *testing.T) {
	app, fm, out := newTestApp(t)
	app.LookPath = func(string) (string, error) { return "", errors.New("not found") }

	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !fm.installDone {
		t.Error("mise.Install() was not called")
	}
	if !strings.Contains(out.String(), "Installing mise") {
		t.Errorf("output = %q, want mise install step", out.String())
	}
}

func TestRunInstallCreatesSymlink(t *testing.T) {
	app, _, _ := newTestApp(t)
	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	target, err := os.Readlink(filepath.Join(app.Home, ".config", "mise", "config.toml"))
	if err != nil {
		t.Fatalf("global config symlink missing: %v", err)
	}
	if target != filepath.Join(app.Home, ".mison", "env", "mise.toml") {
		t.Errorf("symlink target = %q", target)
	}
}

func TestRunUninstallRemovesDeclarationAndTool(t *testing.T) {
	app, fm, _ := newTestApp(t)
	app.In = strings.NewReader("y\n")
	if err := app.RunInstall([]string{"node", "go"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{{Name: "node", Version: "22.11.0"}, {Name: "go", Version: "1.25.1"}}

	if err := app.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}

	toml := readToml(t, app)
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
	app, _, _ := newTestApp(t)
	app.In = strings.NewReader("n\n")
	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}

	if err := app.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	toml := readToml(t, app)
	if !strings.Contains(toml, "node") {
		t.Errorf("node should remain after declined prompt:\n%s", toml)
	}
}

func TestRunUninstallUnknownTool(t *testing.T) {
	app, _, _ := newTestApp(t)
	app.In = strings.NewReader("y\n")
	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	if err := app.RunUninstall([]string{"python"}, false, PolicyAsk); err == nil {
		t.Fatal("RunUninstall() expected error for unknown tool")
	}
}

func TestRunStatusRendersStates(t *testing.T) {
	app, _, out := newTestApp(t)
	if err := app.RunInstall([]string{"node@22", "go@1.25", "python@3.13"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	app.Mise.(*fakeMise).installed = []mise.Tool{
		{Name: "node", Version: "22.11.0"},
		{Name: "go", Version: "1.24.0"},
	}

	if err := app.RunStatus(); err != nil {
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
	app, fm, out := newTestApp(t)
	if err := app.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.execCalls = nil

	if err := app.RunSync(false, PolicyAsk); err != nil {
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
	app, fm, out := newTestApp(t)
	if err := app.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{{Name: "node", Version: "22.11.0"}}
	fm.execCalls = nil

	if err := app.RunSync(false, PolicyAsk); err != nil {
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
	app, fm, _ := newTestApp(t)
	if err := app.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{
		{Name: "node", Version: "22.11.0"},
		{Name: "ripgrep", Version: "14.1.0"},
	}
	fm.execCalls = nil

	if err := app.RunSync(true, PolicyAsk); err != nil {
		t.Fatalf("RunSync(prune) error = %v", err)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "uninstall --all ripgrep") {
		t.Errorf("exec calls = %v, want orphan removal", fm.execCalls)
	}
}

func TestRunSyncWithoutEnvironment(t *testing.T) {
	app, _, _ := newTestApp(t)
	err := app.RunSync(false, PolicyAsk)
	if err == nil || !strings.Contains(err.Error(), "mison init") {
		t.Fatalf("RunSync() error = %v, want init hint", err)
	}
}

func TestRunSyncRestoresGlobalSymlink(t *testing.T) {
	app, fm, _ := newTestApp(t)
	if err := app.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	// simulate a machine that got the env dir by cloning: symlink removed
	if err := os.Remove(filepath.Join(app.Home, ".config", "mise", "config.toml")); err != nil {
		t.Fatal(err)
	}
	fm.execCalls = nil

	if err := app.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if target, err := os.Readlink(filepath.Join(app.Home, ".config", "mise", "config.toml")); err != nil || target == "" {
		t.Fatalf("global symlink not restored by sync: %v", err)
	}
}
