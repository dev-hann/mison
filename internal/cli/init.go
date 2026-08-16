package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// RunInit implements `mison init`: bootstrap the machine into a mison
// environment (mise → gh → private env repo → declaration symlink).
func (a *App) RunInit(repoName string) error {
	r := a.ui()
	info := a.detect()
	r.Step(fmt.Sprintf("Detected %s/%s", info.OS, info.Arch))

	if _, err := a.layout().Ensure(); err != nil {
		return err
	}
	if err := a.ensureMise(); err != nil {
		return err
	}

	if err := a.ensureGh(); err != nil {
		return err
	}

	if err := a.connectRepo(repoName); err != nil {
		return err
	}

	r.Step("Installing declared tools")
	if err := a.Mise.Exec("install"); err != nil {
		return err
	}
	r.Step("Environment ready")
	return nil
}

// ensureGh installs gh via mise, declares it in mise.toml (DESIGN.md:
// gh is part of the environment so every machine bootstraps itself),
// then authenticates and wires git credentials.
func (a *App) ensureGh() error {
	r := a.ui()
	if !a.Gh.IsInstalled() {
		r.Step("Installing gh")
		if err := a.Mise.Exec("install", "gh@latest"); err != nil {
			return err
		}
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	ghDeclared := false
	for _, t := range cfg.Tools() {
		if t.Name == "gh" {
			ghDeclared = true
		}
	}
	if !ghDeclared {
		cfg.SetTool(envTool("gh", "latest"))
		if err := a.saveConfig(cfg); err != nil {
			return err
		}
	}

	if !a.Gh.AuthStatus() {
		a.ui().Line("GitHub login required — follow the browser prompt:")
		if err := a.Gh.AuthLogin(); err != nil {
			return err
		}
	}
	return a.Gh.SetupGit()
}

// connectRepo links ~/.mison/env to the GitHub environment repo:
// local clone → smart pull; remote exists (another machine created it)
// → connect by fetch+reset; otherwise create the private repo, init
// git, and push the initial declaration.
func (a *App) connectRepo(repoName string) error {
	r := a.ui()
	envDir := a.layout().EnvDir
	repo := a.Git(envDir)

	if repo.IsRepo() && repo.RemoteURL() != "" {
		r.Step("Connecting environment")
		_, err := repo.SmartPull(a.makeResolver(PolicyAsk))
		return err
	}

	if a.Gh.RepoExists(repoName) {
		r.Step("Connecting to existing environment repository " + repoName)
		url, err := a.Gh.RepoURL(repoName)
		if err != nil {
			return err
		}
		if err := repo.Connect(url); err != nil {
			return err
		}
		if repo.RemoteIsEmpty() {
			// remote created but never seeded: push the initial state
			if err := writeReadme(envDir, repoName); err != nil {
				return err
			}
			_, err = repo.SmartPush("mison: init environment", a.makeResolver(PolicyAsk))
		}
		return err
	}

	r.Step("Creating environment repository " + repoName)
	url, err := a.Gh.CreatePrivateRepo(repoName)
	if err != nil {
		return err
	}

	if err := writeReadme(envDir, repoName); err != nil {
		return err
	}
	if err := repo.Init(); err != nil {
		return err
	}
	if err := repo.RemoteAdd(url); err != nil {
		return err
	}
	_, err = repo.SmartPush("mison: init environment", a.makeResolver(PolicyAsk))
	return err
}

func writeReadme(envDir, repoName string) error {
	name := filepath.Base(repoName)
	content := fmt.Sprintf("# %s\n\nDevelopment environment managed by [mison](https://github.com/dev-hann/mison).\n\nEdit `mise.toml` (or use the mison CLI) and sync.\n", name)
	return os.WriteFile(filepath.Join(envDir, "README.md"), []byte(content), 0o644)
}
