// Package detector inspects the current system (OS, arch, mise
// presence). Path resolution lives in internal/xdg; this package stays
// pure detection only.
package detector

import "runtime"

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
