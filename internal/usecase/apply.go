package usecase

import (
	"os"
	"strings"

	"github.com/dev-hann/mison/internal/detector"
	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/ui"
)

// ApplyResult is the classified outcome of one tool's apply attempt.
// Apply flows never abort mid-loop on a failing tool — outcomes are
// collected and rendered, and the caller decides the exit policy.
type ApplyResult int

// Apply result classes.
const (
	Applied         ApplyResult = iota // installed (or already satisfied)
	SkippedPlatform                    // no build for this machine (lock or os field)
	Failed                             // mise attempt errored (Detail holds why)
)

// ToolOutcome pairs one declared tool with its apply classification.
type ToolOutcome struct {
	Tool   env.Tool
	Result ApplyResult
	Detail string
}

// machinePlatformKey maps goos/goarch onto mise lock platform keys.
func machinePlatformKey(goos, goarch string) string {
	osName := goos
	if osName == "darwin" {
		osName = "macos"
	}
	arch := goarch
	if arch == "amd64" {
		arch = "x64"
	}
	return osName + "-" + arch
}

// lockPlatforms reads the env repo's mise.lock into a per-tool
// platform set. A missing or unparsable lock yields nil — callers
// treat that as "unknown, try installing".
func (f *Flows) lockPlatforms() map[string][]string {
	data, err := os.ReadFile(f.layout().MiseLock)
	if err != nil || len(data) == 0 {
		return nil
	}
	platforms, err := env.ParseLock(data)
	if err != nil {
		return nil
	}
	return platforms
}

// platformScope splits declared tools into those that should be
// attempted on this machine and skip-outcomes for the rest. A tool is
// skipped when its manual os field excludes this machine, or when the
// lockfile proves the tool publishes no build for this platform.
// Tools absent from the lock stay in scope (fallback: try).
func (f *Flows) platformScope(declared []env.Tool) ([]env.Tool, []ToolOutcome) {
	platforms := f.lockPlatforms()
	info := f.detect()
	key := machinePlatformKey(info.OS, info.Arch)

	var inScope []env.Tool
	var skipped []ToolOutcome
	for _, t := range declared {
		if len(t.OS) > 0 && !t.AppliesTo(info.OS, info.Arch) {
			skipped = append(skipped, ToolOutcome{
				Tool: t, Result: SkippedPlatform,
				Detail: "restricted to " + strings.Join(t.OS, ", "),
			})
			continue
		}
		if supported, ok := platforms[t.Name]; ok && !contains(supported, key) {
			skipped = append(skipped, ToolOutcome{
				Tool: t, Result: SkippedPlatform,
				Detail: "available for " + strings.Join(supported, ", "),
			})
			continue
		}
		inScope = append(inScope, t)
	}
	return inScope, skipped
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// attemptTool installs a declared tool at its config version; mise's
// native progress streams (ExecTTY) framed by an in-flight marker.
func (f *Flows) attemptTool(t env.Tool) ToolOutcome {
	f.UI.Line("→ " + t.Name)
	if err := f.Mise.ExecTTY("install", t.Name); err != nil {
		return ToolOutcome{Tool: t, Result: Failed, Detail: err.Error()}
	}
	return ToolOutcome{Tool: t, Result: Applied}
}

// attemptSpec installs an explicit name@version (the install flow —
// nothing is declared yet, so the version rides the command line).
func (f *Flows) attemptSpec(name, version string) ToolOutcome {
	tool := env.Tool{Name: name, Version: version}
	f.UI.Line("→ " + name)
	if err := f.Mise.ExecTTY("install", name+"@"+version); err != nil {
		return ToolOutcome{Tool: tool, Result: Failed, Detail: err.Error()}
	}
	return ToolOutcome{Tool: tool, Result: Applied}
}

// reportOutcomes renders one line per tool and returns the number of
// Failed entries.
func (f *Flows) reportOutcomes(outcomes []ToolOutcome) int {
	failed := 0
	for _, o := range outcomes {
		switch o.Result {
		case Applied:
			f.UI.ToolLine(ui.MarkOK, o.Tool.Name, o.Tool.Version)
		case SkippedPlatform:
			f.UI.ToolLine(ui.MarkWarning, o.Tool.Name, "not for this platform ("+o.Detail+")")
		case Failed:
			failed++
			f.UI.ToolLine(ui.MarkFail, o.Tool.Name, o.Detail)
		}
	}
	return failed
}

// detectInfo is the machine context for platform scoping.
var _ = detector.Detect
