// Package detector inspects the current system (OS, arch, mise presence).
package detector

import "runtime"

// Info describes the current machine.
type Info struct {
	OS   string // "darwin" or "linux"
	Arch string // "arm64" or "amd64"
}

// Detect returns information about the current system.
func Detect() Info {
	return Info{OS: runtime.GOOS, Arch: runtime.GOARCH}
}
