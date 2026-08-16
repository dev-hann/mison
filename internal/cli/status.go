package cli

import (
	"fmt"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/ui"
)

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
	a.renderSyncStatus()
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
