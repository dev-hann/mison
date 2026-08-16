package cli

import (
	"fmt"
	"strings"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/ui"
)

// Reporter is the one-way notification port: business flows report
// what happened through it and never read anything back. The keek-news
// "dumb widget" equivalent — flows own no rendering knowledge.
type Reporter interface {
	Step(msg string)   // ✓ completed local action
	Synced(msg string) // ↻ remote merge notice (always shown)
	Warn(msg string)   // ⚠ non-fatal issue
	Fail(msg string)   // ✗ fatal issue
	Line(msg string)   // plain output
	ToolLine(mark, name, detail string)
}

// Prompter is the two-way confirmation port: blocking questions whose
// answers gate destructive or ambiguous steps.
type Prompter interface {
	Confirm(question string) bool                     // y/N gate
	ResolveConflict(c env.Conflict) (env.Tool, error) // [1/2] gate
}

// TermUI implements Reporter and Prompter on a terminal, sharing the
// app's lazy buffered reader so consecutive prompts never lose input.
type TermUI struct {
	app *App
}

// NewTermUI builds the terminal interaction adapter for an app.
func NewTermUI(app *App) *TermUI { return &TermUI{app: app} }

func (t *TermUI) r() *ui.Renderer { return t.app.ui() }

// Step implements Reporter.
func (t *TermUI) Step(msg string) { t.r().Step(msg) }

// Synced implements Reporter.
func (t *TermUI) Synced(msg string) { t.r().Synced(msg) }

// Warn implements Reporter.
func (t *TermUI) Warn(msg string) { t.r().Warn(msg) }

// Fail implements Reporter.
func (t *TermUI) Fail(msg string) { t.r().Fail(msg) }

// Line implements Reporter.
func (t *TermUI) Line(msg string) { t.r().Line(msg) }

// ToolLine implements Reporter.
func (t *TermUI) ToolLine(mark, name, detail string) {
	t.r().ToolLine(mark, name, detail)
}

// Confirm implements Prompter.
func (t *TermUI) Confirm(question string) bool {
	_, _ = fmt.Fprintf(t.app.Stdout, "%s %s [y/N] ", ui.MarkWarning, question)
	switch strings.ToLower(strings.TrimSpace(t.app.readLine())) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// ResolveConflict implements Prompter.
func (t *TermUI) ResolveConflict(c env.Conflict) (env.Tool, error) {
	localDesc := "removed"
	if c.Local.Name != "" {
		localDesc = c.Local.Version
	}
	remoteDesc := "removed"
	if c.Remote.Name != "" {
		remoteDesc = c.Remote.Version
	}
	t.Fail(fmt.Sprintf("Conflict on %s (this machine: %s, remote: %s)", c.Name, localDesc, remoteDesc))
	t.Line("  [1] keep this machine  [2] accept remote")
	_, _ = fmt.Fprint(t.app.Stdout, "Choose [1/2]: ")

	switch strings.TrimSpace(t.app.readLine()) {
	case "1":
		return pickSide(c.Local, c.Remote), nil
	case "2":
		return pickSide(c.Remote, c.Local), nil
	default:
		return env.Tool{}, fmt.Errorf("conflict on %s unresolved — aborting (local changes kept unpushed)", c.Name)
	}
}
