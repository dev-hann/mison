// Package detector inspects the current system (OS, arch, mise presence).
// All filesystem/environment access is injected so tests stay pure.
package detector

import (
	"path/filepath"
	"runtime"

	"github.com/dev-hann/mison/internal/xdg"
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

// ShimPath returns the mise shims directory for home (XDG-aware).
func ShimPath(home string) string {
	return xdg.MiseShims(home)
}
