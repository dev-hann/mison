// Package mise wraps the mise CLI as mison's installation engine.
package mise

// Manager controls mise lifecycle and tool installation.
// Implementations must inject mise shim paths into PATH when
// executing commands (see docs/ARCHITECTURE.md).
type Manager interface {
	IsInstalled() bool
	Version() (string, error)
	Install() error
	Exec(args ...string) error
}
