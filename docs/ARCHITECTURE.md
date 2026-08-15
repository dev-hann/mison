# Mison Architecture

## Package structure

```
cmd/mison/          main() only — thin entrypoint
internal/cli/       cobra commands; argument parsing; orchestration calls.
                    NO business logic. Delegates to managers.
internal/ui/        output rendering: marks (✓↻✗⚠), lines, prompts.
                    Only package allowed to write user-facing output.
internal/detector/  pure system detection: OS, arch, mise presence.
internal/env/       mise.toml parsing, editing, diffing, semantic 3-way merge.
                    PURE: no os/exec, no filesystem writes (paths injected).
internal/mise/      mise engine wrapper. Manager interface + real impl.
internal/gitclient/ git operations on the env repo. Client interface +
                    exec-based impl; integration-tested with t.TempDir().
```

## Dependency rules

```
cli → ui, detector, env, mise, gitclient
mise → ui (progress), detector
env  → (nothing internal; BurntSushi/toml only)
gitclient → env (semantic merge on conflict)
```

- `env` and `detector` stay pure (testable without mocks of OS facilities).
- Interfaces are defined in the consuming style: `mise.Manager`,
  `gitclient.Client` are injected into `cli` handlers; tests provide fakes.
- `internal/ui` owns ALL user output — commands never fmt.Print directly.

## Key interfaces

```go
type MiseManager interface {
    IsInstalled() bool
    Version() (string, error)
    Install() error            // via mise.run script
    Exec(args ...string) error // PATH prepended with mise shims
}

type GitClient interface {
    Commit(message string) error
    Push() error               // fetch+rebase on rejection, per DESIGN.md
    PullRebase() error         // semantic merge hook on conflict
}
```

## Execution details

- mise invocation: `exec.Command` with `~/.local/share/mise/shims` (and
  `~/.local/bin`) prepended to PATH. No shell activation required.
- Env layout: `~/.mison/env/` = git clone; `~/.config/mise/config.toml`
  symlinked to `~/.mison/env/mise.toml` so mise itself reads the same file.
- TOML writes: preserve unknown sections; comment loss on rewrite accepted (V1).

## Data flow: install

```
cli.install(tools, osFlag)
  → env.AddTools(declaration, tools, osFlag)   (pure, returns new decl)
  → env.Write(miseTomlPath, decl)              (single write point)
  → mise.Exec("install")
  → gitclient.Commit("install: ..."), Push()   (fetch→rebase→push policy)
  → ui.OK / ui.Synced(mergedTools) / ui.Fail
```

## Testing strategy

| Layer | Approach |
|---|---|
| env (parse/diff/merge) | table-driven unit tests, strict TDD |
| detector, ui | unit tests, golden files for ui |
| mise, gitclient | interface fakes for cli tests; real-git integration tests in t.TempDir() |
| cli wiring | test-after, cobra Execute against fakes |
| e2e (mise.run, gh) | `//go:build e2e`, CI-only |
