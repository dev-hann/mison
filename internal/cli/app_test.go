package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func newTestApp(t *testing.T) (*App, *fakeMise, *bytes.Buffer) {
	t.Helper()
	fm := &fakeMise{}
	out := &bytes.Buffer{}
	app := &App{
		Home:     t.TempDir(),
		Stdout:   out,
		In:       strings.NewReader(""),
		Mise:     fm,
		LookPath: foundLookPath,
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

	if err := app.RunInstall([]string{"node@22", "go"}, ""); err != nil {
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

	if err := app.RunInstall([]string{"docker"}, "linux"); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	toml := readToml(t, app)
	if !strings.Contains(toml, `os = ["linux"]`) {
		t.Errorf("mise.toml missing os restriction:\n%s", toml)
	}
}

func TestRunInstallInvalidSpec(t *testing.T) {
	app, _, _ := newTestApp(t)
	if err := app.RunInstall([]string{"node@"}, ""); err == nil {
		t.Fatal("RunInstall() expected error for node@")
	}
}

func TestRunInstallInstallsMiseWhenMissing(t *testing.T) {
	app, fm, out := newTestApp(t)
	app.LookPath = func(string) (string, error) { return "", errors.New("not found") }

	if err := app.RunInstall([]string{"node"}, ""); err != nil {
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
	if err := app.RunInstall([]string{"node"}, ""); err != nil {
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
	if err := app.RunInstall([]string{"node", "go"}, ""); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{{Name: "node", Version: "22.11.0"}, {Name: "go", Version: "1.25.1"}}

	if err := app.RunUninstall([]string{"node"}, false); err != nil {
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
	if err := app.RunInstall([]string{"node"}, ""); err != nil {
		t.Fatal(err)
	}

	if err := app.RunUninstall([]string{"node"}, false); err != nil {
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
	if err := app.RunInstall([]string{"node"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := app.RunUninstall([]string{"python"}, false); err == nil {
		t.Fatal("RunUninstall() expected error for unknown tool")
	}
}

func TestRunStatusRendersStates(t *testing.T) {
	app, _, out := newTestApp(t)
	if err := app.RunInstall([]string{"node@22", "go@1.25", "python@3.13"}, ""); err != nil {
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
	if err := app.RunInstall([]string{"node@22"}, ""); err != nil {
		t.Fatal(err)
	}
	fm.execCalls = nil

	if err := app.RunSync(false); err != nil {
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
	if err := app.RunInstall([]string{"node@22"}, ""); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{{Name: "node", Version: "22.11.0"}}
	fm.execCalls = nil

	if err := app.RunSync(false); err != nil {
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
	if err := app.RunInstall([]string{"node@22"}, ""); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{
		{Name: "node", Version: "22.11.0"},
		{Name: "ripgrep", Version: "14.1.0"},
	}
	fm.execCalls = nil

	if err := app.RunSync(true); err != nil {
		t.Fatalf("RunSync(prune) error = %v", err)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "uninstall --all ripgrep") {
		t.Errorf("exec calls = %v, want orphan removal", fm.execCalls)
	}
}

func TestRunSyncWithoutEnvironment(t *testing.T) {
	app, _, _ := newTestApp(t)
	err := app.RunSync(false)
	if err == nil || !strings.Contains(err.Error(), "mison init") {
		t.Fatalf("RunSync() error = %v, want init hint", err)
	}
}
