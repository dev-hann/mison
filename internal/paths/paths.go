// Package paths owns the mison on-disk layout:
//
//	~/.mison/env/            environment repository clone (M2) / local file (M1)
//	~/.mison/env/mise.toml   the shared declaration
//	~/.config/mise/config.toml  symlink → declaration, so mise reads
//	                            the same file mison manages.
package paths

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dev-hann/mison/internal/xdg"
)

// Layout describes every path mison touches for a given home directory.
type Layout struct {
	EnvDir       string
	MiseToml     string
	GlobalConfig string
}

// New resolves the layout under the default XDG config dir.
func New(home string) Layout {
	return NewEnv(home, xdg.ConfigDir(home))
}

// NewEnv resolves the layout with an explicit XDG_CONFIG_HOME value.
func NewEnv(home, xdgConfig string) Layout {
	configDir := filepath.Join(home, ".config")
	if xdgConfig != "" {
		configDir = xdgConfig
	}
	envDir := filepath.Join(home, ".mison", "env")
	return Layout{
		EnvDir:       envDir,
		MiseToml:     filepath.Join(envDir, "mise.toml"),
		GlobalConfig: filepath.Join(configDir, "mise", "config.toml"),
	}
}

// Ensure creates ~/.mison/env, an empty mise.toml when absent, and the
// global-config symlink. It reports whether anything was created.
// Existing declarations are never modified.
func (l Layout) Ensure() (created bool, err error) {
	if err := os.MkdirAll(l.EnvDir, 0o755); err != nil {
		return false, fmt.Errorf("create env dir: %w", err)
	}
	if _, statErr := os.Stat(l.MiseToml); os.IsNotExist(statErr) {
		if err := os.WriteFile(l.MiseToml, []byte(""), 0o644); err != nil {
			return false, fmt.Errorf("create mise.toml: %w", err)
		}
		created = true
	} else if statErr != nil {
		return false, fmt.Errorf("stat mise.toml: %w", statErr)
	}

	linkCreated, err := l.ensureSymlink()
	if err != nil {
		return created, err
	}
	return created || linkCreated, nil
}

func (l Layout) ensureSymlink() (bool, error) {
	if info, err := os.Lstat(l.GlobalConfig); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if target, _ := os.Readlink(l.GlobalConfig); target == l.MiseToml {
				return false, nil
			}
		}
		// foreign file or wrong symlink target: replace
		if err := os.Remove(l.GlobalConfig); err != nil {
			return false, fmt.Errorf("replace global config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat global config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(l.GlobalConfig), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.Symlink(l.MiseToml, l.GlobalConfig); err != nil {
		return false, fmt.Errorf("symlink global config: %w", err)
	}
	return true, nil
}
