package cli

import (
	"fmt"
	"strings"
)

// RunUninstall implements `mison uninstall <tools...> [--yes]`.
func (a *App) RunUninstall(args []string, assumeYes bool, policy ConflictPolicy) error {
	if !assumeYes && !a.Ask.Confirm(fmt.Sprintf("Remove %s from the environment?", strings.Join(args, ", "))) {
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
			a.UI.Step(fmt.Sprintf("Removing %s", name))
			if err := a.Mise.Exec("uninstall", "--all", name); err != nil {
				return err
			}
		} else {
			a.UI.Step(fmt.Sprintf("Removed %s (not installed)", name))
		}
	}
	a.commitAndPush(fmt.Sprintf("uninstall: %s", strings.Join(args, ", ")), policy)
	return nil
}
