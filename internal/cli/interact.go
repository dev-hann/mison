package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/dev-hann/mison/internal/env"
	"github.com/dev-hann/mison/internal/ui"
	"github.com/dev-hann/mison/internal/usecase"
)

// TermUI implements usecase.Reporter and usecase.Prompter on a
// terminal. It owns a single buffered reader so consecutive prompts
// never lose buffered input.
type TermUI struct {
	out    io.Writer
	in     io.Reader
	reader *bufio.Reader
}

// NewTermUI builds the terminal interaction adapter.
func NewTermUI(out io.Writer, in io.Reader) *TermUI {
	return &TermUI{out: out, in: in}
}

func (t *TermUI) r() *ui.Renderer { return ui.New(t.out) }

func (t *TermUI) readLine() string {
	if t.reader == nil {
		t.reader = bufio.NewReader(t.in)
	}
	line, err := t.reader.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return line
}

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
	_, _ = fmt.Fprintf(t.out, "%s %s [y/N] ", ui.MarkWarning, question)
	switch strings.ToLower(strings.TrimSpace(t.readLine())) {
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
	t.Line("  [1] keep this machine  [2] accept remote  [3] abort (keep unpushed)")
	_, _ = fmt.Fprint(t.out, "Choose [1/2/3]: ")

	switch strings.TrimSpace(t.readLine()) {
	case "1":
		return usecase.PickSide(c.Local, c.Remote), nil
	case "2":
		return usecase.PickSide(c.Remote, c.Local), nil
	default:
		return env.Tool{}, fmt.Errorf("conflict on %s unresolved — aborting (local changes kept unpushed)", c.Name)
	}
}
