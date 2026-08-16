// Package mise wraps the mise CLI as mison's installation engine.
package mise

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dev-hann/mison/internal/xdg"
)

// Tool mirrors an installed tool reported by mise.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Executor runs a command with a custom environment (os/exec wrapper).
type Executor interface {
	Run(env []string, name string, args ...string) (string, error)
}

// Manager controls mise lifecycle and tool installation.
type Manager interface {
	IsInstalled() bool
	Version() (string, error)
	Install() error
	Exec(args ...string) error
	InstalledTools() ([]Tool, error)
}

// RealManager is the default Manager backed by a real Executor.
type RealManager struct {
	exec Executor
	home string
}

// NewManager builds a mise manager bound to the user's home directory.
func NewManager(exec Executor, home string) *RealManager {
	return &RealManager{exec: exec, home: home}
}

// IsInstalled reports whether mise responds to --version.
func (m *RealManager) IsInstalled() bool {
	_, err := m.exec.Run(m.env(), "mise", "--version")
	return err == nil
}

// Version returns the first token of `mise --version` output.
func (m *RealManager) Version() (string, error) {
	out, err := m.exec.Run(m.env(), "mise", "--version")
	if err != nil {
		return "", fmt.Errorf("mise version: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("mise version: empty output")
	}
	return fields[0], nil
}

// Install runs the official mise.run installer script.
func (m *RealManager) Install() error {
	out, err := m.exec.Run(m.env(), "sh", "-c", "curl -fsSL https://mise.run | sh")
	if err != nil {
		return fmt.Errorf("install mise: %w — %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Exec runs an arbitrary mise command with shims on PATH.
func (m *RealManager) Exec(args ...string) error {
	out, err := m.exec.Run(m.env(), "mise", args...)
	if err != nil {
		return fmt.Errorf("mise %s: %w — %s", strings.Join(args, " "), err, strings.TrimSpace(out))
	}
	return nil
}

// InstalledTools returns tools currently active (`mise ls --current --json`)
// that are declared by mison's own config (the global symlink or the
// ~/.mison/env/mise.toml declaration). Tools from project configs or
// foreign global configs are excluded.
func (m *RealManager) InstalledTools() ([]Tool, error) {
	out, err := m.exec.Run(m.env(), "mise", "ls", "--current", "--json")
	if err != nil {
		return nil, fmt.Errorf("mise ls: %w", err)
	}

	var ls map[string][]lsEntry
	if err := json.Unmarshal([]byte(out), &ls); err != nil {
		return nil, fmt.Errorf("mise ls: parse output: %w", err)
	}

	ours := m.ownConfigPaths()
	tools := make([]Tool, 0, len(ls))
	for name, entries := range ls {
		for _, e := range entries {
			if e.Active && ours.has(e.Source.Path) {
				tools = append(tools, Tool{Name: name, Version: e.Version})
				break
			}
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

type lsEntry struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	Source    struct {
		Type string `json:"type"`
		Path string `json:"path"`
	} `json:"source"`
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

func (m *RealManager) ownConfigPaths() pathSet {
	global := filepath.Join(xdg.ConfigDir(m.home), "mise", "config.toml")
	decl := filepath.Join(m.home, ".mison", "env", "mise.toml")

	set := pathSet{}
	for _, p := range []string{global, decl} {
		set[filepath.Clean(p)] = struct{}{}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			set[resolved] = struct{}{}
		}
	}
	return set
}

// env builds the execution environment: shim dir and mise bin dir
// prepended to PATH so mison never depends on shell activation.
func (m *RealManager) env() []string {
	path := xdg.MiseShims(m.home) + ":" + xdg.MiseBin(m.home)
	if cur := os.Getenv("PATH"); cur != "" {
		path += ":" + cur
	}

	env := []string{"PATH=" + path}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	return env
}
