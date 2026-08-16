// Package xdg is the single source of truth for XDG-aware paths that
// mison derives (config/data dirs, mise shims and binary locations).
// Env-reading wrappers live here so every package resolves identically.
package xdg

import (
	"os"
	"path/filepath"
)

// ConfigDir resolves the user config directory (XDG_CONFIG_HOME or ~/.config).
func ConfigDir(home string) string {
	return ConfigDirEnv(home, os.Getenv("XDG_CONFIG_HOME"))
}

// ConfigDirEnv is ConfigDir with an explicit env value (pure).
func ConfigDirEnv(home, xdgConfig string) string {
	if xdgConfig != "" {
		return xdgConfig
	}
	return filepath.Join(home, ".config")
}

// DataDir resolves the user data directory (XDG_DATA_HOME or ~/.local/share).
func DataDir(home string) string {
	return DataDirEnv(home, os.Getenv("XDG_DATA_HOME"))
}

// DataDirEnv is DataDir with an explicit env value (pure).
func DataDirEnv(home, xdgData string) string {
	if xdgData != "" {
		return xdgData
	}
	return filepath.Join(home, ".local", "share")
}

// MiseShims returns the mise shims directory for home.
func MiseShims(home string) string {
	return filepath.Join(DataDir(home), "mise", "shims")
}

// MiseBin returns the mise binary install directory for home (mise.run).
func MiseBin(home string) string {
	return filepath.Join(home, ".local", "bin")
}
