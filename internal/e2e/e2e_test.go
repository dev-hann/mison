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
