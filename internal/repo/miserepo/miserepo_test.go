package miserepo

import (
	"fmt"
	"strings"
	"testing"
)

// fakeRunner scripts command results and records invocations.
type fakeRunner struct {
	calls   []string
	envs    [][]string
	results map[string]string
	errs    map[string]error
}

func (f *fakeRunner) Run(env []string, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	f.envs = append(f.envs, env)
	if err, ok := f.errs[key]; ok {
		return "", err
	}
	out, ok := f.results[key]
	if !ok {
		return "", fmt.Errorf("unexpected command: %s", key)
	}
	return out, nil
}

func (f *fakeRunner) RunTTY(env []string, name string, args ...string) error {
	_, err := f.Run(env, name, args...)
	return err
}

func TestIsInstalled(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{"mise --version": "2026.1.1 linux-x64\n"}}
	m := New(fr, "/home/u")

	if !m.IsInstalled() {
		t.Fatal("IsInstalled() = false, want true")
	}
}

func TestVersionTrimsOutput(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{"mise --version": "2026.1.1 linux-x64\n"}}
	m := New(fr, "/home/u")

	v, err := m.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if v != "2026.1.1" {
		t.Fatalf("Version() = %q, want 2026.1.1 (first token, trimmed)", v)
	}
}

func TestRunInstallerCommandShape(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"sh -c curl -fsSL https://mise.run | sh": "mise: installed\n",
	}}
	m := New(fr, "/home/u")

	if err := m.RunInstaller(); err != nil {
		t.Fatalf("RunInstaller() error = %v", err)
	}
	if len(fr.calls) != 1 || !strings.HasPrefix(fr.calls[0], "sh -c curl") {
		t.Errorf("calls = %v, want mise.run installer", fr.calls)
	}
}

func TestExecPrependsShimPath(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{"mise install": ""}}
	m := New(fr, "/home/u")

	if err := m.Exec("install"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}

	env := fr.envs[0]
	var path string
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			path = strings.TrimPrefix(kv, "PATH=")
		}
	}
	if !strings.HasPrefix(path, "/home/u/.local/share/mise/shims:") {
		t.Fatalf("PATH = %q, want shims dir prepended", path)
	}
	if !strings.Contains(path, "/home/u/.local/bin") {
		t.Fatalf("PATH = %q, want mise bin dir included", path)
	}
}

func TestExecErrorPropagates(t *testing.T) {
	fr := &fakeRunner{errs: map[string]error{
		"mise install node@22": fmt.Errorf("no version found"),
	}}
	m := New(fr, "/home/u")

	if err := m.Exec("install", "node@22"); err == nil {
		t.Fatal("Exec() expected error")
	}
}

func TestListInstalledParsesRawEntries(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"mise ls --current --json": `{
  "node": [
    {"version": "20.1.0", "active": false,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}},
    {"version": "22.23.2", "active": true,
     "source": {"type": "mise.toml", "path": "/home/u/.config/mise/config.toml"}}
  ],
  "go": [
    {"version": "1.26.6", "active": true,
     "source": {"type": "mise.toml", "path": "/home/hann/.config/mise/config.toml"}}
  ]
}`,
	}}
	m := New(fr, "/home/u")

	entries, err := m.ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled() error = %v", err)
	}
	// raw view: every version entry, no activity/ownership filtering
	if len(entries) != 3 {
		t.Fatalf("entries = %+v, want 3 raw entries", entries)
	}
	if entries[0].Name != "go" || entries[0].Source == "" {
		t.Errorf("go entry = %+v, want source preserved", entries[0])
	}
	if !entries[2].Active || entries[2].Version != "22.23.2" {
		t.Errorf("node active entry = %+v", entries[2])
	}
}

func TestBumpDryRunParsesCandidates(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"mise lock --global --bump --dry-run --json": `[
  {"name": "node", "backend": "core:node", "lockfile": "~/.config/mise/mise.lock",
   "old_versions": ["22.23.2"], "new_versions": ["22.24.0"]},
  {"name": "jq", "backend": "core:jq", "lockfile": "~/.config/mise/mise.lock",
   "old_versions": ["1.8.2"], "new_versions": ["1.9.0"]}
]`,
	}}
	m := New(fr, "/home/u")

	cs, err := m.BumpDryRun()
	if err != nil {
		t.Fatalf("BumpDryRun() error = %v", err)
	}
	if len(cs) != 2 || cs[0].Name != "node" ||
		cs[0].OldVersions[0] != "22.23.2" || cs[0].NewVersions[0] != "22.24.0" {
		t.Fatalf("candidates = %+v", cs)
	}
}

func TestBumpDryRunEmpty(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"mise lock --global --bump --dry-run --json": "[]",
	}}
	m := New(fr, "/home/u")

	cs, err := m.BumpDryRun()
	if err != nil || len(cs) != 0 {
		t.Fatalf("empty bump = %+v, %v — want zero candidates, no error", cs, err)
	}
}

func TestDoctorCollectsProblems(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"mise doctor": `version: 2026.9.1
activated: no
shims_on_path: no

1 problem found:
1. mise is not activated, run mise help activate or
   add the shims directory to PATH.
`,
	}}
	m := New(fr, "/home/u")

	problems := m.Doctor()
	if len(problems) != 1 || !strings.Contains(problems[0], "not activated") {
		t.Fatalf("Doctor() = %v, want one activation problem", problems)
	}
}

func TestDoctorHealthyIsEmpty(t *testing.T) {
	fr := &fakeRunner{results: map[string]string{
		"mise doctor": "version: 2026.9.1\nNo problems found\n",
	}}
	m := New(fr, "/home/u")

	if got := m.Doctor(); len(got) != 0 {
		t.Fatalf("Doctor() = %v, want none", got)
	}
}
