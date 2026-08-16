package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutUsesMisonDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	l := New("/home/u")
	if l.EnvDir != "/home/u/.mison/env" {
		t.Errorf("EnvDir = %q", l.EnvDir)
	}
	if l.MiseToml != filepath.Join(l.EnvDir, "mise.toml") {
		t.Errorf("MiseToml = %q", l.MiseToml)
	}
	if l.GlobalConfig != "/home/u/.config/mise/config.toml" {
		t.Errorf("GlobalConfig = %q", l.GlobalConfig)
	}
}

func TestLayoutRespectsXDGConfig(t *testing.T) {
	l := NewEnv("/home/u", "/xdg/config")
	if l.GlobalConfig != "/xdg/config/mise/config.toml" {
		t.Errorf("GlobalConfig = %q, want XDG path", l.GlobalConfig)
	}
}

func TestEnsureCreatesEnvDirAndFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	l := New(t.TempDir())

	created, err := l.Ensure()
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !created {
		t.Fatal("Ensure() created = false, want true on fresh layout")
	}

	info, err := os.Stat(l.MiseToml)
	if err != nil || info.IsDir() {
		t.Fatalf("mise.toml missing after Ensure(): %v", err)
	}
	data, _ := os.ReadFile(l.MiseToml)
	if string(data) != "" {
		t.Fatalf("fresh mise.toml = %q, want empty", data)
	}
}

func TestEnsureIdempotent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	l := New(t.TempDir())
	if _, err := l.Ensure(); err != nil {
		t.Fatalf("first Ensure(): %v", err)
	}
	if err := os.WriteFile(l.MiseToml, []byte("[tools]\nnode = \"22\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created, err := l.Ensure()
	if err != nil {
		t.Fatalf("second Ensure(): %v", err)
	}
	if created {
		t.Fatal("Ensure() created = true on existing layout, want false")
	}
	data, _ := os.ReadFile(l.MiseToml)
	if string(data) != "[tools]\nnode = \"22\"\n" {
		t.Fatalf("existing mise.toml was modified: %q", data)
	}
}

func TestEnsureSymlinksGlobalConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	l := New(home)

	if _, err := l.Ensure(); err != nil {
		t.Fatalf("Ensure(): %v", err)
	}

	info, err := os.Lstat(l.GlobalConfig)
	if err != nil {
		t.Fatalf("global config missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("global config is not a symlink: mode=%v", info.Mode())
	}
	target, _ := os.Readlink(l.GlobalConfig)
	if target != l.MiseToml {
		t.Fatalf("symlink target = %q, want %q", target, l.MiseToml)
	}
}

func TestEnsureReplacesForeignFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	l := New(home)

	if err := os.MkdirAll(filepath.Dir(l.GlobalConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.GlobalConfig, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := l.Ensure(); err != nil {
		t.Fatalf("Ensure(): %v", err)
	}
	info, err := os.Lstat(l.GlobalConfig)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("foreign file should be replaced by symlink: %v", err)
	}
}
