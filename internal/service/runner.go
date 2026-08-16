// Package service is mison's process boundary: the only place that
// spawns external commands (git, gh, mise, sh). Layers above depend on
// the Runner interface and never touch os/exec directly.
package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dev-hann/mison/internal/xdg"
)

// Runner executes external commands. Run captures combined output and
// returns errors that include stderr detail; RunTTY streams the child's
// stdio to the terminal for interactive commands (gh device flow).
type Runner interface {
	Run(env []string, name string, args ...string) (string, error)
	RunTTY(env []string, name string, args ...string) error
}

// OsRunner is the os/exec-backed Runner.
type OsRunner struct{}

// New returns the real Runner.
func New() Runner { return OsRunner{} }

// Run implements Runner with combined output capture.
func (OsRunner) Run(env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(out.String())
		if detail == "" {
			return out.String(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return out.String(), fmt.Errorf("%s %s: %w — %s", name, strings.Join(args, " "), err, detail)
	}
	return out.String(), nil
}

// RunTTY implements Runner for interactive commands; the child process
// owns the terminal (stdin/stdout/stderr passthrough).
func (OsRunner) RunTTY(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// MiseEnv builds the environment with mise shims and the mise binary
// directory prepended to PATH, so commands run without shell
// activation and gh resolves right after `mise install gh`.
func MiseEnv(home string) []string {
	return WithPath(home, os.Getenv("PATH"))
}

// WithPath returns the current environment with mise paths prepended
// to the given PATH value (pure — reads env only for passthrough).
func WithPath(home, path string) []string {
	misePath := xdg.MiseShims(home) + ":" + xdg.MiseBin(home)
	if path != "" {
		misePath += ":" + path
	}
	env := []string{"PATH=" + misePath}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	return env
}
