package mise

import (
	"fmt"
	"strings"
	"testing"
)

// fakeExec records every invocation and replays scripted results.
type fakeExec struct {
	calls     []string              // "name arg1 arg2" per invocation
	envByCall map[int][]string      // env recorded per call index
	results   map[string]execResult // key: "name args..." → result
	callIdx   int
}

type execResult struct {
	stdout string
	err    error
}

func newFakeExec() *fakeExec {
	return &fakeExec{envByCall: map[int][]string{}, results: map[string]execResult{}}
}

func (f *fakeExec) script(key, stdout string, err error) {
	f.results[key] = execResult{stdout: stdout, err: err}
}

func (f *fakeExec) Run(env []string, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	f.envByCall[f.callIdx] = env
	f.callIdx++
	r, ok := f.results[key]
	if !ok {
		return "", fmt.Errorf("unexpected command: %s", key)
	}
	return r.stdout, r.err
}

func TestIsInstalled(t *testing.T) {
	fx := newFakeExec()
	fx.script("mise --version", "2026.1.1 linux-x64\n", nil)
	m := NewManager(fx, "/home/u")

	if !m.IsInstalled() {
		t.Fatal("IsInstalled() = false, want true")
	}
	if got := fx.calls[0]; got != "mise --version" {
		t.Errorf("call = %q, want 'mise --version'", got)
	}
}

func TestIsInstalledWhenMissing(t *testing.T) {
	fx := newFakeExec()
	fx.script("mise --version", "", fmt.Errorf("exit status 127"))
	m := NewManager(fx, "/home/u")

	if m.IsInstalled() {
		t.Fatal("IsInstalled() = true, want false")
	}
}

func TestVersionTrimsOutput(t *testing.T) {
	fx := newFakeExec()
	fx.script("mise --version", "2026.1.1 linux-x64\n", nil)
	m := NewManager(fx, "/home/u")

	v, err := m.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if v != "2026.1.1" {
		t.Fatalf("Version() = %q, want 2026.1.1 (first token, trimmed)", v)
	}
}

func TestInstallRunsOfficialScript(t *testing.T) {
	fx := newFakeExec()
	fx.script("sh -c curl -fsSL https://mise.run | sh", "mise: installed\n", nil)
	m := NewManager(fx, "/home/u")

	if err := m.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestExecPrependsShimPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	fx := newFakeExec()
	fx.script("mise install", "", nil)
	m := NewManager(fx, "/home/u")

	if err := m.Exec("install"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	env := fx.envByCall[0]
	path := lookupEnv(env, "PATH")
	if !strings.HasPrefix(path, "/home/u/.local/share/mise/shims:") {
		t.Fatalf("PATH = %q, want shims dir prepended", path)
	}
	if !strings.Contains(path, "/home/u/.local/bin") {
		t.Fatalf("PATH = %q, want mise bin dir included", path)
	}
}

func TestExecErrorPropagates(t *testing.T) {
	fx := newFakeExec()
	fx.script("mise install node@22", "", fmt.Errorf("no version found"))
	m := NewManager(fx, "/home/u")

	if err := m.Exec("install", "node@22"); err == nil {
		t.Fatal("Exec() expected error")
	}
}

func TestInstalledToolsParsesLS(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	fx := newFakeExec()
	fx.script("mise ls --current --json", `{
  "node": [
    {"version": "22.23.2", "installed": true, "active": true,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}}
  ],
  "go": [
    {"version": "1.25.1", "installed": true, "active": true,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}}
  ]
}`, nil)
	m := NewManager(fx, "/home/u")

	tools, err := m.InstalledTools()
	if err != nil {
		t.Fatalf("InstalledTools() error = %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "go" || tools[1].Name != "node" || tools[1].Version != "22.23.2" {
		t.Fatalf("tools = %+v, want sorted [go node]", tools)
	}
}

func TestInstalledToolsFiltersForeignSources(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	// tools declared by other config files (project mise.toml, foreign
	// global config) must not leak into mison's view
	fx := newFakeExec()
	fx.script("mise ls --current --json", `{
  "node": [
    {"version": "22.23.2", "installed": true, "active": true,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}}
  ],
  "go": [
    {"version": "1.26.6", "installed": true, "active": true,
     "source": {"type": "mise.toml", "path": "/home/hann/.config/mise/config.toml"}}
  ]
}`, nil)
	m := NewManager(fx, "/home/u")

	tools, err := m.InstalledTools()
	if err != nil {
		t.Fatalf("InstalledTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "node" {
		t.Fatalf("tools = %+v, want only node (ours)", tools)
	}
}

func TestInstalledToolsMatchesDeclarationPathDirectly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	// source may point at the real declaration file instead of the symlink
	fx := newFakeExec()
	fx.script("mise ls --current --json", `{
  "node": [
    {"version": "22.23.2", "installed": true, "active": true,
     "source": {"type": "mise.toml", "path": "/home/u/.mison/env/mise.toml"}}
  ]
}`, nil)
	m := NewManager(fx, "/home/u")

	tools, err := m.InstalledTools()
	if err != nil {
		t.Fatalf("InstalledTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "node" {
		t.Fatalf("tools = %+v, want node via direct path", tools)
	}
}

func TestInstalledToolsPicksActiveEntry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	// multiple installed versions: the active one counts
	fx := newFakeExec()
	fx.script("mise ls --current --json", `{
  "node": [
    {"version": "20.1.0", "installed": true, "active": false,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}},
    {"version": "22.23.2", "installed": true, "active": true,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}}
  ]
}`, nil)
	m := NewManager(fx, "/home/u")

	tools, err := m.InstalledTools()
	if err != nil {
		t.Fatalf("InstalledTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Version != "22.23.2" {
		t.Fatalf("tools = %+v, want active 22.23.2", tools)
	}
}

func lookupEnv(env []string, key string) string {
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			return strings.TrimPrefix(kv, key+"=")
		}
	}
	return ""
}
