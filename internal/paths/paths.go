// Package paths owns the mison on-disk layout:
//
//	~/.mison/env/            environment repository clone (M2) / local file (M1)
//	~/.mison/env/mise.toml   the shared declaration
//	~/.mison/env/mise.lock   the derived lockfile (regenerated, never merged)
//	~/.config/mise/config.toml  symlink → declaration, so mise reads
//	                            the same file mison manages.
//	~/.config/mise/mise.lock    symlink → lockfile, so `mise lock`
//	                            writes into the environment repository.
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
	MiseLock     string
	GlobalConfig string
	GlobalLock   string
	RunLock      string // run mutex — outside the repo so it never commits
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
		MiseLock:     filepath.Join(envDir, "mise.lock"),
		GlobalConfig: filepath.Join(configDir, "mise", "config.toml"),
		GlobalLock:   filepath.Join(configDir, "mise", "mise.lock"),
		RunLock:      filepath.Join(home, ".mison", ".run.lock"),
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

	linkCreated, err := l.ensureSymlink(l.GlobalConfig, l.MiseToml)
	if err != nil {
		return created, err
	}
	lockCreated, err := l.ensureSymlink(l.GlobalLock, l.MiseLock)
	if err != nil {
		return created, err
	}
	return created || linkCreated || lockCreated, nil
}

func (l Layout) ensureSymlink(path, target string) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if cur, _ := os.Readlink(path); cur == target {
				return false, nil
			}
		}
		// foreign file or wrong symlink target: replace. A real file
		// (a standalone mise user's config/lock) is backed up first —
		// its content predates mison and is not ours to destroy.
		if info.Mode()&os.ModeSymlink == 0 {
			if err := os.Rename(path, path+".mison-bak"); err != nil {
				return false, fmt.Errorf("back up %s: %w", filepath.Base(path), err)
			}
		} else if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("replace %s: %w", filepath.Base(path), err)
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", filepath.Base(path), err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.Symlink(target, path); err != nil {
		return false, fmt.Errorf("symlink %s: %w", filepath.Base(path), err)
	}
	return true, nil
}
