// Package cli wires up the mison command tree.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
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
