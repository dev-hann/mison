// Package miserepo provides atomic mise commands over an injected
// service.Runner. It returns raw data only — ownership filtering is
// usecase policy.
package miserepo

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dev-hann/mison/internal/service"
)

// Entry is one installed-tool record from `mise ls --current --json`.
// Source is the config file that declared the tool (ownership signal).
type Entry struct {
	Name    string
	Version string
	Active  bool
	Source  string
}

// Repo runs atomic mise commands with shims on PATH.
type Repo struct {
	r    service.Runner
	home string
}

// New builds a mise Repo bound to the user's home directory.
func New(r service.Runner, home string) *Repo { return &Repo{r: r, home: home} }

func (m *Repo) exec(args ...string) (string, error) {
	return m.r.Run(service.MiseEnv(m.home), "mise", args...)
}

// IsInstalled reports whether mise responds to --version.
func (m *Repo) IsInstalled() bool {
	_, err := m.exec("--version")
	return err == nil
}

// Version returns the first token of `mise --version`.
func (m *Repo) Version() (string, error) {
	out, err := m.exec("--version")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("mise version: empty output")
	}
	return fields[0], nil
}

// RunInstaller runs the official mise.run installer script.
func (m *Repo) RunInstaller() error {
	out, err := m.r.Run(service.MiseEnv(m.home), "sh", "-c", "curl -fsSL https://mise.run | sh")
	if err != nil {
		return fmt.Errorf("install mise: %w — %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Exec runs an arbitrary mise command (install, uninstall, prune...).
// The Runner error is already self-describing (command + stderr) — no
// re-prefixing.
func (m *Repo) Exec(args ...string) error {
	_, err := m.exec(args...)
	return err
}

// BumpCandidate is one fuzzy-selector re-resolution offered by
// `mise lock --bump --dry-run --json`.
type BumpCandidate struct {
	Name        string   `json:"name"`
	OldVersions []string `json:"old_versions"`
	NewVersions []string `json:"new_versions"`
}

// BumpDryRun lists available updates without writing anything.
func (m *Repo) BumpDryRun() ([]BumpCandidate, error) {
	out, err := m.exec("lock", "--global", "--bump", "--dry-run", "--json")
	if err != nil {
		return nil, fmt.Errorf("mise lock --bump --dry-run: %w", err)
	}
	var candidates []BumpCandidate
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &candidates); err != nil {
		return nil, fmt.Errorf("mise lock --bump --dry-run: parse output: %w", err)
	}
	return candidates, nil
}

// Doctor returns mise's self-check problem list (empty = healthy).
// Output is captured even on non-zero exit — problems are the signal,
// not the exit code.
func (m *Repo) Doctor() []string {
	out, _ := m.r.Run(service.MiseEnv(m.home), "mise", "doctor")
	var problems []string
	inList := false
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "problem") && strings.Contains(line, "found") {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && trimmed[1] == '.' {
			problems = append(problems, trimmed[3:])
		}
	}
	return problems
}

// ListInstalled parses `mise ls --current --json` into raw entries.
// Every installed version is reported; filtering by activity or
// ownership belongs to the caller.
func (m *Repo) ListInstalled() ([]Entry, error) {
	out, err := m.exec("ls", "--current", "--json")
	if err != nil {
		return nil, err
	}

	var ls map[string][]lsEntry
	if err := json.Unmarshal([]byte(out), &ls); err != nil {
		return nil, fmt.Errorf("mise ls: parse output: %w", err)
	}

	entries := make([]Entry, 0, len(ls))
	for name, list := range ls {
		for _, e := range list {
			entries = append(entries, Entry{
				Name:    name,
				Version: e.Version,
				Active:  e.Active,
				Source:  e.Source.Path,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

type lsEntry struct {
	Version string `json:"version"`
	Active  bool   `json:"active"`
	Source  struct {
		Type string `json:"type"`
		Path string `json:"path"`
	} `json:"source"`
}
