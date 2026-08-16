// Package cli wires up the mison command tree.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/dev-hann/mison/internal/detector"
	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/mise"
	"github.com/dev-hann/mison/internal/paths"
	"github.com/dev-hann/mison/internal/ui"
)

// App carries the dependencies every command handler needs.
// All fields are injected; tests provide fakes.
type App struct {
	Home      string
	Stdout    io.Writer
	In        io.Reader
	Mise      mise.Manager
	LookPath  detector.LookPathFunc
	Git       func(dir string) Repo
	Gh        GhClient
	bufReader *bufio.Reader
}

func (a *App) ui() *ui.Renderer     { return ui.New(a.Stdout) }
func (a *App) layout() paths.Layout { return paths.New(a.Home) }
func (a *App) detect() detector.Info {
	return detector.Detect()
}

// reader lazily wraps In in a single buffered reader so consecutive
// prompts never lose buffered input.
func (a *App) reader() *bufio.Reader {
	if a.bufReader == nil {
		a.bufReader = bufio.NewReader(a.In)
	}
	return a.bufReader
}

func (a *App) readLine() string {
	line, err := a.reader().ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return line
}

// confirm asks a yes/no question.
func (a *App) confirm(question string) bool {
	_, _ = fmt.Fprintf(a.Stdout, "%s %s [y/N] ", ui.MarkWarning, question)
	switch strings.ToLower(strings.TrimSpace(a.readLine())) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func envTool(name, version string) env.Tool {
	return env.Tool{Name: name, Version: version}
}

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
	for _, spec := range args {
		name, version, err := env.ParseToolSpec(spec)
		if err != nil {
			return err
		}
		cfg.SetTool(env.Tool{Name: name, Version: version, OS: osSpec})
		names = append(names, name)
	}
	if err := a.saveConfig(cfg); err != nil {
		return err
	}

	a.ui().Step(fmt.Sprintf("Installing %s", strings.Join(names, ", ")))
	if err := a.Mise.Exec("install"); err != nil {
		return err
	}
	a.commitAndPush(fmt.Sprintf("install: %s", strings.Join(names, ", ")), policy)
	return nil
}

// RunUninstall implements `mison uninstall <tools...> [--yes]`.
func (a *App) RunUninstall(args []string, assumeYes bool, policy ConflictPolicy) error {
	if !assumeYes && !a.confirm(fmt.Sprintf("Remove %s from the environment?", strings.Join(args, ", "))) {
		return nil
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

	installed := map[string]bool{}
	if tools, err := a.Mise.InstalledTools(); err == nil {
		for _, t := range tools {
			installed[t.Name] = true
		}
	}

	var missing []string
	for _, name := range args {
		if !cfg.RemoveTool(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("not in environment: %s", strings.Join(missing, ", "))
	}
	if err := a.saveConfig(cfg); err != nil {
		return err
	}

	for _, name := range args {
		if installed[name] {
			a.ui().Step(fmt.Sprintf("Removing %s", name))
			if err := a.Mise.Exec("uninstall", "--all", name); err != nil {
				return err
			}
		} else {
			a.ui().Step(fmt.Sprintf("Removed %s (not installed)", name))
		}
	}
	a.commitAndPush(fmt.Sprintf("uninstall: %s", strings.Join(args, ", ")), policy)
	return nil
}

// RunStatus implements `mison status`.
func (a *App) RunStatus() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	installed, err := a.Mise.InstalledTools()
	if err != nil {
		return err
	}

	declared := cfg.Tools()
	diff := env.Diff(declared, toEnvTools(installed))

	r := a.ui()
	r.Line("Environment status")
	var missing int
	for _, st := range diff {
		switch st.State {
		case env.StateOK:
			r.ToolLine(ui.MarkOK, st.Tool.Name, st.Tool.Version)
		case env.StateMissing:
			r.ToolLine(ui.MarkFail, st.Tool.Name, "missing — run mison sync")
			missing++
		case env.StateMismatch:
			r.ToolLine(ui.MarkWarning, st.Tool.Name,
				fmt.Sprintf("declared %s, installed %s", st.Tool.Version, st.Installed))
		}
	}
	if len(diff) == 0 {
		r.Line("No tools declared.")
	}
	if missing > 0 {
		r.Warn(fmt.Sprintf("%d tool(s) missing", missing))
	}
	return nil
}

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

func (a *App) ensureMise() error {
	if detector.IsMiseInstalled(a.LookPath) {
		return nil
	}
	a.ui().Step("Installing mise")
	return a.Mise.Install()
}

func (a *App) loadConfig() (*env.Config, error) {
	data, err := os.ReadFile(a.layout().MiseToml)
	if err != nil {
		return nil, fmt.Errorf("read mise.toml: %w", err)
	}
	return env.Parse(data)
}

func (a *App) saveConfig(cfg *env.Config) error {
	data, err := cfg.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.layout().MiseToml, data, 0o644); err != nil {
		return fmt.Errorf("write mise.toml: %w", err)
	}
	return nil
}

func toEnvTools(tools []mise.Tool) []env.Tool {
	out := make([]env.Tool, len(tools))
	for i, t := range tools {
		out[i] = env.Tool{Name: t.Name, Version: t.Version}
	}
	return out
}
