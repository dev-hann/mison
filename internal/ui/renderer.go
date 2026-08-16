// Package ui renders user-facing output (checkmarks, warnings, errors).
// It is the only package that writes user-facing output.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Status markers used across mison output.
const (
	MarkOK      = "✓"
	MarkSync    = "↻"
	MarkFail    = "✗"
	MarkWarning = "⚠"
)

// Renderer writes formatted output to an injected writer.
type Renderer struct {
	w io.Writer
}

// New builds a Renderer.
func New(w io.Writer) *Renderer {
	return &Renderer{w: w}
}

func (r *Renderer) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(r.w, format, a...)
}

// Step reports a completed local action.
func (r *Renderer) Step(msg string) {
	r.printf("%s %s\n", MarkOK, msg)
}

// Synced reports a remote merge that the user must always see.
func (r *Renderer) Synced(msg string) {
	r.printf("%s %s\n", MarkSync, msg)
}

// Warn reports a non-fatal issue.
func (r *Renderer) Warn(msg string) {
	r.printf("%s %s\n", MarkWarning, msg)
}

// Fail reports a fatal issue.
func (r *Renderer) Fail(msg string) {
	r.printf("%s %s\n", MarkFail, msg)
}

// Line writes plain output.
func (r *Renderer) Line(msg string) {
	r.printf("%s\n", msg)
}

// ToolLine renders one tool status row.
func (r *Renderer) ToolLine(mark, name, detail string) {
	if detail == "" {
		r.printf("%s %s\n", mark, name)
		return
	}
	r.printf("%s %s (%s)\n", mark, name, detail)
}

// Prompt asks a yes/no question and reads one line from in.
// It returns false when input is unavailable (non-interactive).
func Prompt(in io.Reader, w io.Writer, question string) bool {
	_, _ = fmt.Fprintf(w, "%s %s [y/N] ", MarkWarning, question)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		_, _ = fmt.Fprintln(w, "")
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
