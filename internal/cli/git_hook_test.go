package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev-hann/mison/internal/mise"
)

func TestInstallPushesWhenRepoConnected(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	app, _, out := newTestAppWith(t, repo)

	if err := app.RunInstall([]string{"node@22"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "install: node" {
		t.Errorf("pushes = %v", repo.pushes)
	}
	if !strings.Contains(out.String(), "✓ Installing node") {
		t.Errorf("output = %q", out.String())
	}
}

func TestInstallShowsRemoteMergeNotice(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"go"}}
	app, _, out := newTestAppWith(t, repo)

	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !strings.Contains(out.String(), "↻ Remote had new changes (go) — merged automatically") {
		t.Errorf("missing ↻ notice:\n%s", out.String())
	}
}

func TestInstallDeferredPushOnFailure(t *testing.T) {
	repo := &fakeRepo{isRepo: true, pushErr: errors.New("network unreachable")}
	app, _, out := newTestAppWith(t, repo)

	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatalf("RunInstall() should not fail when push fails: %v", err)
	}
	if !strings.Contains(out.String(), "could not push") {
		t.Errorf("missing deferred-push warning:\n%s", out.String())
	}
}

func TestUninstallPushes(t *testing.T) {
	repo := &fakeRepo{isRepo: true}
	app, fm, _ := newTestAppWith(t, repo)
	app.In = strings.NewReader("y\n")
	if err := app.RunInstall([]string{"node"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	fm.installed = []mise.Tool{{Name: "node", Version: "22.1.0"}}
	repo.pushes = nil

	if err := app.RunUninstall([]string{"node"}, false, PolicyAsk); err != nil {
		t.Fatalf("RunUninstall() error = %v", err)
	}
	if len(repo.pushes) != 1 || repo.pushes[0] != "uninstall: node" {
		t.Errorf("pushes = %v", repo.pushes)
	}
}

func TestSyncPullsBeforeApply(t *testing.T) {
	repo := &fakeRepo{isRepo: true, mergedOn: []string{"node"}}
	app, fm, out := newTestAppWith(t, repo)
	if err := app.RunInstall([]string{"rg"}, "", PolicyAsk); err != nil {
		t.Fatal(err)
	}
	repo.pushes = nil
	fm.execCalls = nil

	if err := app.RunSync(false, PolicyAsk); err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if repo.pulls != 1 {
		t.Errorf("pulls = %d, want 1", repo.pulls)
	}
	if !strings.Contains(out.String(), "↻ New changes: node") {
		t.Errorf("missing pull notice:\n%s", out.String())
	}
}

func TestRunInitCreatesRepoAndPushes(t *testing.T) {
	repo := &fakeRepo{}
	app, fm, out := newTestAppWith(t, repo)
	app.Gh = &fakeGh{installed: false, authed: false}
	ghc := app.Gh.(*fakeGh)

	if err := app.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}

	if len(ghc.created) != 1 || ghc.created[0] != "mison-environment" {
		t.Errorf("created repos = %v", ghc.created)
	}
	if repo.remote == "" {
		t.Error("remote not set")
	}
	if len(repo.pushes) == 0 {
		t.Error("initial push missing")
	}

	tomlData, err := os.ReadFile(filepath.Join(app.Home, ".mison", "env", "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tomlData), "gh") {
		t.Errorf("gh must be declared in mise.toml:\n%s", tomlData)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "install gh@latest") {
		t.Errorf("gh install call missing: %v", fm.execCalls)
	}
	if !strings.Contains(out.String(), "Environment ready") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunInitConnectsExistingRepo(t *testing.T) {
	repo := &fakeRepo{isRepo: true, remote: "https://github.com/me/env.git"}
	app, _, _ := newTestAppWith(t, repo)
	ghc := app.Gh.(*fakeGh)

	if err := app.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	if len(ghc.created) != 0 {
		t.Errorf("should not create when connected, created = %v", ghc.created)
	}
	if repo.pulls != 1 {
		t.Errorf("pulls = %d, want 1", repo.pulls)
	}
}

func TestRunInitGhNotInstalledFlow(t *testing.T) {
	repo := &fakeRepo{}
	app, fm, _ := newTestAppWith(t, repo)
	app.Gh = &fakeGh{installed: false, authed: false}

	if err := app.RunInit("mison-environment"); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}
	joined := strings.Join(fm.execCalls, ";")
	if !strings.Contains(joined, "install gh@latest") {
		t.Errorf("gh bootstrap missing: %v", fm.execCalls)
	}
	if !app.Gh.(*fakeGh).authed {
		t.Error("auth login not performed")
	}
}
