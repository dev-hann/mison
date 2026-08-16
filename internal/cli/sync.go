package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/gitclient"
	"github.com/dev-hann/mison/internal/ui"
)

// RunSync implements `mison sync`: pull declaration (when the env repo
// is connected), apply it via mise, prune orphans on request.
func (a *App) RunSync(prune bool, policy ConflictPolicy) error {
	if _, err := os.Stat(a.layout().MiseToml); err != nil {
		return fmt.Errorf("no environment found — run mison init first")
	}
	// restore the global-config symlink: machines that received the env
	// by cloning (or lost the symlink) must still be seen by mise
	if _, err := a.layout().Ensure(); err != nil {
		return err
	}
	if err := a.ensureMise(); err != nil {
		return err
	}

	if repo := a.Git(a.layout().EnvDir); repo.IsRepo() {
		a.ui().Step("Pulling environment")
		merged, err := repo.SmartPull(a.makeResolver(policy))
		if err != nil {
			a.ui().Warn("pull failed — continuing with local declaration (" + err.Error() + ")")
		} else if len(merged) > 0 {
			a.ui().Synced(fmt.Sprintf("New changes: %s", strings.Join(merged, ", ")))
		}
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	declared := cfg.Tools()
	installed, err := a.Mise.InstalledTools()
	if err != nil {
		return err
	}

	diff := env.Diff(declared, toEnvTools(installed))
	var needsApply bool
	for _, st := range diff {
		if st.State != env.StateOK {
			needsApply = true
			break
		}
	}
	if needsApply {
		a.ui().Step("Installing declared tools")
		if err := a.Mise.Exec("install"); err != nil {
			return err
		}
		a.verifyDeclaredApplied(declared)
	}

	declaredNames := map[string]bool{}
	for _, t := range declared {
		declaredNames[t.Name] = true
	}
	var orphans []string
	for _, t := range installed {
		if !declaredNames[t.Name] {
			orphans = append(orphans, t.Name)
		}
	}
	sort.Strings(orphans)

	r := a.ui()
	switch {
	case len(orphans) == 0:
		// nothing extra
	case prune:
		for _, name := range orphans {
			r.Step(fmt.Sprintf("Pruning %s", name))
			if err := a.Mise.Exec("uninstall", "--all", name); err != nil {
				return err
			}
		}
	default:
		if a.confirm(fmt.Sprintf("Remove undeclared tools (%s)?", strings.Join(orphans, ", "))) {
			for _, name := range orphans {
				r.Step(fmt.Sprintf("Pruning %s", name))
				if err := a.Mise.Exec("uninstall", "--all", name); err != nil {
					return err
				}
			}
		} else {
			r.Warn("kept (run mison sync --prune to remove automatically)")
		}
	}

	if needsApply || len(orphans) > 0 {
		r.Step("Environment synchronized")
	} else {
		r.Step("Already synchronized")
	}
	return nil
}

// renderSyncStatus prints the local-vs-GitHub declaration relation.
func (a *App) renderSyncStatus() {
	r := a.ui()
	repo := a.Git(a.layout().EnvDir)
	if !repo.IsRepo() {
		r.Line("Sync: not connected — run mison init to link GitHub")
		return
	}
	info, err := repo.SyncStatus()
	if err != nil {
		r.Warn("could not compare with GitHub (" + err.Error() + ")")
		return
	}
	switch info.State {
	case gitclient.SyncUpToDate:
		r.Step("up to date with GitHub")
	case gitclient.SyncBehind:
		r.ToolLine(ui.MarkSync, "remote has new tools", strings.Join(info.RemoteAdded, ", ")+" — run mison sync")
	case gitclient.SyncAhead:
		r.ToolLine(ui.MarkWarning, "local changes not pushed", "run mison sync")
	case gitclient.SyncDiverged:
		r.ToolLine(ui.MarkWarning, "diverged from GitHub", "local and remote both changed — run mison sync")
	}
}
