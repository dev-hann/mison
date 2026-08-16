// Package detector inspects the current system (OS, arch, mise presence).
// All filesystem/environment access is injected so tests stay pure.
package detector

import (
	"os"
	"path/filepath"
	"runtime"
)

// LookPathFunc resolves an executable name to a path (os/exec.LookPath).
type LookPathFunc func(name string) (string, error)

// Info describes the current machine.
type Info struct {
	OS   string // "darwin" or "linux" on V1
	Arch string // "arm64" or "amd64"
}

// Detect returns information about the current system.
func Detect() Info {
	return Info{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// IsMiseInstalled reports whether a mise binary is resolvable on PATH.
func IsMiseInstalled(lookPath LookPathFunc) bool {
	_, err := lookPath("mise")
	return err == nil
}

// MiseBinaryPath returns the standard mise.run install location for home.
func MiseBinaryPath(home string) string {
	return filepath.Join(home, ".local", "bin", "mise")
}

// ShimPath returns the default mise shims directory for home.
func ShimPath(home string) string {
	return ShimPathEnv(home, os.Getenv("XDG_DATA_HOME"))
}

// ShimPathEnv resolves the shims directory, honoring XDG_DATA_HOME
// when set (mise: $XDG_DATA_HOME/mise/shims, else ~/.local/share/mise/shims).
func ShimPathEnv(home, xdgData string) string {
	base := filepath.Join(home, ".local", "share")
	if xdgData != "" {
		base = xdgData
	}
	return filepath.Join(base, "mise", "shims")
}
