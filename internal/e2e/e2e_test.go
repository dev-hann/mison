//go:build e2e

// Package e2e holds end-to-end tests that hit the real world: the mise
// installer script and the gh CLI. Run with: go test -tags e2e ./...
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/repo/gitrepo"
	"github.com/dev-hann/mison/internal/repo/miserepo"
	"github.com/dev-hann/mison/internal/service"
)

func TestMiseRunInstallsIntoTempHome(t *testing.T) {
	home := t.TempDir()
	script := "curl -fsSL https://mise.run | sh"
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mise.run installer failed: %v\n%s", err, out)
	}
	bin := filepath.Join(home, ".local", "bin", "mise")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("mise binary missing at %s", bin)
	}
}

func TestMiseRepoAgainstRealMise(t *testing.T) {
	home := os.Getenv("HOME")
	m := miserepo.New(service.New(), home)

	if !m.IsInstalled() {
		t.Fatal("mise not installed on this machine")
	}
	if _, err := m.Version(); err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if _, err := m.ListInstalled(); err != nil {
		t.Fatalf("ListInstalled() error = %v", err)
	}
}

func TestGhAuthAndRepoURL(t *testing.T) {
	home := os.Getenv("HOME")
	c := gitrepo.NewGitHub(service.New(), home)
	if !c.IsInstalled() {
		t.Skip("gh not installed")
	}
	if !c.AuthStatus() {
		t.Skip("gh not authenticated")
	}
	url, err := c.RepoURL("dev-hann/mison")
	if err != nil {
		t.Fatalf("RepoURL() error = %v", err)
	}
	if !strings.HasSuffix(url, "dev-hann/mison.git") {
		t.Fatalf("RepoURL() = %q", url)
	}
}

func TestLockGlobalDeterministic(t *testing.T) {
	miseBin := filepath.Join(os.Getenv("HOME"), ".local", "bin", "mise")
	if _, err := os.Stat(miseBin); err != nil {
		t.Skip("mise not installed on this machine")
	}

	home := t.TempDir()
	envDir := filepath.Join(home, ".mison", "env")
	cfgDir := filepath.Join(home, ".config", "mise")
	work := filepath.Join(home, "work")
	for _, d := range []string{envDir, cfgDir, filepath.Join(home, ".local", "share", "mise"), work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	toml := filepath.Join(envDir, "mise.toml")
	if err := os.WriteFile(toml, []byte("[tools]\njq = \"latest\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalConfig := filepath.Join(cfgDir, "config.toml")
	if err := os.Symlink(toml, globalConfig); err != nil {
		t.Fatal(err)
	}
	globalLock := filepath.Join(cfgDir, "mise.lock")
	// dangling symlink exactly as mison's paths.Ensure creates it
	if err := os.Symlink(filepath.Join(envDir, "mise.lock"), globalLock); err != nil {
		t.Fatal(err)
	}

	// mise does not fully honor $HOME on macOS (its migrate scan reads
	// the OS-home default config regardless), so isolate every root via
	// the documented env contract and whitelist the OS-home default
	// config path — otherwise a mison-dogfooding machine's real config
	// leaks into the isolated run as untrusted and fails it.
	realHome, _ := os.UserHomeDir()
	cmdEnv := []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"MISE_GLOBAL_CONFIG_FILE=" + globalConfig,
		"MISE_GLOBAL_CONFIG_ROOT=" + home,
		"MISE_DATA_DIR=" + filepath.Join(home, ".local", "share", "mise"),
		"MISE_CACHE_DIR=" + filepath.Join(home, ".cache", "mise"),
		"MISE_TRUSTED_CONFIG_PATHS=" + globalConfig + ":" + filepath.Join(realHome, ".config", "mise", "config.toml"),
		"MISE_CEILING_PATHS=" + home,
	}

	lock := func() string {
		cmd := exec.Command(miseBin, "lock", "--global")
		cmd.Dir = work
		cmd.Env = cmdEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("mise lock: %v\n%s", err, out)
		}
		data, err := os.ReadFile(globalLock)
		if err != nil {
			t.Fatalf("read lock: %v", err)
		}
		return string(data)
	}

	first := lock()
	// pin the behavior mison's refreshLock adopts around: mise lock
	// replaces the symlink with a regular file (atomic rename)
	if info, err := os.Lstat(globalLock); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected mise lock to clobber the symlink with a regular file: %v", err)
	}
	second := lock()
	if first != second {
		t.Fatal("mise lock --global must be byte-deterministic across runs")
	}
	if strings.Contains(strings.ToLower(first), "timestamp") {
		t.Fatal("lockfile must not embed timestamps (would break cross-machine convergence)")
	}
}

func TestMiseInstallOneOffDoesNotDeclare(t *testing.T) {
	miseBin := filepath.Join(os.Getenv("HOME"), ".local", "bin", "mise")
	if _, err := os.Stat(miseBin); err != nil {
		t.Skip("mise not installed on this machine")
	}

	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "mise")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalConfig := filepath.Join(cfgDir, "config.toml")
	before := "[tools]\njq = \"latest\"\n"
	if err := os.WriteFile(globalConfig, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	// apply-first load-bearing assumption: `mise install name@version`
	// is a one-off — it must never write the config
	cmd := exec.Command(miseBin, "install", "jq@latest")
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"MISE_GLOBAL_CONFIG_FILE=" + globalConfig,
		"MISE_GLOBAL_CONFIG_ROOT=" + home,
		"MISE_DATA_DIR=" + filepath.Join(home, ".local", "share", "mise"),
		"MISE_CACHE_DIR=" + filepath.Join(home, ".cache", "mise"),
		"MISE_TRUSTED_CONFIG_PATHS=" + globalConfig,
		"MISE_CEILING_PATHS=" + home,
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mise install jq: %v\n%s", err, out)
	}
	after, err := os.ReadFile(globalConfig)
	if err != nil || string(after) != before {
		t.Fatalf("mise install name@version must not modify config: %v\n%s", err, after)
	}
}

// TestInstallerWiresShellPath runs the real installer against a temp
// HOME (zsh) and asserts the rc wiring: appended once, idempotent on
// rerun, and absent with --skip-shell / CI.
func TestInstallerWiresShellPath(t *testing.T) {
	// run the LOCAL script — this test guards uncommitted changes too
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(wd, "..", "..", "scripts", "install.sh")
	home := t.TempDir()
	rc := filepath.Join(home, ".zshrc")
	env := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "SHELL=/bin/zsh", "CI="}

	run := func(extra ...string) {
		args := append([]string{script}, extra...)
		cmd := exec.Command("sh", args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("installer: %v\n%s", err, out)
		}
	}

	// CI= in env means NOT CI — append must happen
	run()
	data, err := os.ReadFile(rc)
	if err != nil || !strings.Contains(string(data), `export PATH="$HOME/.local/bin:$PATH"`) {
		t.Fatalf("rc must gain the PATH block: %v\n%s", err, data)
	}

	// rerun: idempotent
	run()
	data, _ = os.ReadFile(rc)
	if got := strings.Count(string(data), "mison (installer)"); got != 1 {
		t.Fatalf("block must appear exactly once, got %d:\n%s", got, data)
	}

	// pre-wired rc: no append
	os.Remove(rc)
	if err := os.WriteFile(rc, []byte("export PATH=\"$HOME/.local/bin:$PATH\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run()
	data, _ = os.ReadFile(rc)
	if strings.Contains(string(data), "mison (installer)") {
		t.Fatalf("already-wired rc must stay untouched:\n%s", data)
	}

	// --skip-shell: no rc file at all
	os.Remove(rc)
	run("--skip-shell")
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatal("--skip-shell must not create the rc file")
	}
}
