package cli

import (
	"fmt"
	"strings"

	"github.com/dev-hann/mison/internal/env"
)

// RunInstall implements `mison install <tools...> [--os-spec]`.
func (a *App) RunInstall(args []string, osFlag string, policy ConflictPolicy) error {
	osSpec := env.ParseOSSpec(osFlag)
	if osFlag != "" && osSpec == nil {
		return fmt.Errorf("invalid OS spec %q (use mac, linux, linux/x64, linux/arm64, macos/arm64)", osFlag)
	}

	if _, err := a.layout().Ensure(); err != nil {
		return err
	}
	if err := a.ensureMise(); err != nil {
		return err
	}

	cfg, err := a.loadConfig()
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
	if err := a.saveConfig(cfg); err != nil {
		return err
	}

	info := a.detect()
	skipped := map[string]bool{}
	for _, t := range tools {
		if len(t.OS) > 0 && !t.AppliesTo(info.OS, info.Arch) {
			skipped[t.Name] = true
			a.ui().Warn(fmt.Sprintf("%s: restricted to %s — skipped on this machine (%s/%s)",
				t.Name, strings.Join(t.OS, ", "), info.OS, info.Arch))
		}
	}

	a.ui().Step(fmt.Sprintf("Installing %s", strings.Join(names, ", ")))
	if err := a.Mise.Exec("install"); err != nil {
		return err
	}
	a.verifyVisible(names, skipped)
	a.commitAndPush(fmt.Sprintf("install: %s", strings.Join(names, ", ")), policy)
	return nil
}

// verifyVisible warns when tools mison just installed are not reported
// by mise — catches silent no-ops (e.g. broken global-config symlink).
func (a *App) verifyVisible(names []string, ignore map[string]bool) {
	installed, err := a.Mise.InstalledTools()
	if err != nil {
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
		a.ui().Warn(fmt.Sprintf("%s not visible to mise — declaration saved; run mison status to check", name))
	}
}

// verifyDeclaredApplied re-checks the declaration after sync applied it;
// OS-restricted tools that do not target this machine are exempt.
func (a *App) verifyDeclaredApplied(declared []env.Tool) {
	installed, err := a.Mise.InstalledTools()
	if err != nil {
		return
	}
	info := a.detect()
	present := make(map[string]bool, len(installed))
	for _, t := range installed {
		present[t.Name] = true
	}
	for _, d := range declared {
		if !d.AppliesTo(info.OS, info.Arch) {
			continue
		}
		if !present[d.Name] {
			a.ui().Warn(fmt.Sprintf("%s not visible to mise — run mison status to check", d.Name))
		}
	}
}
