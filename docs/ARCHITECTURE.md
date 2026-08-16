# Mison Architecture

## Layers

```
cli → usecase → repo → service
```

Each layer may only reference the layer below (plus pure leaves:
env, paths, xdg, detector, ui). Skipping is prohibited.

```
cmd/mison/            main() only — thin entrypoint, version via ldflags
internal/cli/         cobra wiring + TermUI adapter. NO logic.
internal/usecase/     business flows, sync policy, interaction ports
internal/repo/gitrepo/  atomic git + gh commands over a Runner
internal/repo/miserepo/ atomic mise commands over a Runner (raw data)
internal/service/     Runner interface + os/exec impl — the process boundary
internal/env/         mise.toml parsing, editing, diffing, semantic merge (pure)
internal/detector/    OS/arch/mise-presence detection (pure)
internal/xdg/         XDG-aware path resolution (config/data, mise shims)
internal/paths/       ~/.mison/env layout + global-config symlink
internal/ui/          mark renderer (✓↻✗⚠) — leaf used by TermUI
internal/e2e/         real-world tests (build tag `e2e`)
```

## Dependency rules

```
✅ cli      → usecase
✅ usecase  → repo/{gitrepo,miserepo}, service, env, paths, xdg, detector, ui
✅ repo     → service (gitrepo also → nothing else; miserepo → nothing else)
❌ service  → any internal package except xdg (path helpers for MiseEnv)
❌ usecase  → service.Runner directly (always through a repo)
❌ repo     → env/usecase/cli (repos are command recipes, not policy)
```

- `service` is the ONLY package that imports `os/exec`. One Runner
  serves git, gh, mise, and `sh -c` (the mise.run installer pipe).
- Interfaces are defined at the consumer: `usecase.EnvRepoIface`,
  `usecase.MiseRepoIface`, `usecase.GhClient` are satisfied by
  `*usecase.Engine`, `*miserepo.Repo`, `*gitrepo.GitHub`; tests
  provide fakes. Repos and service return concrete types.

## Key interfaces

```go
// service
type Runner interface {
    Run(env []string, name string, args ...string) (string, error) // self-describing errors
    RunTTY(env []string, name string, args ...string) error        // gh device flow
}

// usecase — interaction ports
type Reporter interface { Step, Synced, Warn, Fail, Line, ToolLine(...) }
type Prompter interface {
    Confirm(question string) bool
    ResolveConflict(c env.Conflict) (env.Tool, error)
}

// usecase — sync policy
func PlanSync(head, remote, base string, hasRemote bool) SyncPlan
//   → SeedRemote | Push | FastForward | Merge (pure, table-tested)
type Engine struct{ ... } // SmartPush/SmartPull/SyncStatus/Connect over gitrepo

// usecase — flows
type Flows struct { Home; UI Reporter; Ask Prompter; Mise; Look; Git; Gh }
```

## Responsibility boundaries

| Concern | Owner |
|---|---|
| Spawning processes, stderr capture, PATH assembly | service |
| Command syntax (git/gh/mise invocations) | repos |
| Raw data shapes (ls --json entries) | miserepo |
| Ownership filtering (our config sources only) | usecase.OwnedTools |
| Sync decisions, semantic merge, conflict policy | usecase (PlanSync + Engine) |
| Command flows, error classification, confirm-gates | usecase.Flows |
| Terminal rendering + prompts | cli.TermUI |
| mise.toml semantics | env |

## Execution details

- mise/gh invocation: Runner runs with mise shims + `~/.local/bin`
  prepended to PATH (service.MiseEnv). No shell activation required.
- Env layout: `~/.mison/env/` = git clone; `~/.config/mise/config.toml`
  symlinked to `~/.mison/env/mise.toml` so mise itself reads the same
  file mison manages.
- TOML writes: preserve unknown sections; comment loss on rewrite
  accepted (V1).
- Reset ordering: behind-remotes are parsed BEFORE `reset --hard` so an
  unreadable remote never mutates the worktree.

## Data flow: install

```
cli.install(tools, osFlag)
  → usecase.RunInstall
      env.ParseToolSpec / SetTool          (pure declaration edit)
      paths.Ensure                         (env dir + symlink)
      miserepo.Exec("install")             (via service.Runner)
      OwnedTools(ListInstalled)            (visibility verification)
      Engine.SmartPush                     (fetch → PlanSync →
                                            semantic merge → push)
      UI.Step / UI.Synced / Ask.ResolveConflict (ports)
```

## Testing strategy

| Layer | Approach |
|---|---|
| env (parse/diff/merge) | table-driven unit tests, strict TDD |
| service | thin; covered via repos and e2e |
| repos | fake Runner scripting (`mise install` → output) |
| usecase.PlanSync + OwnedTools | pure table tests |
| usecase.Engine | real-git integration in t.TempDir() (sync_test.go) |
| usecase.Flows | fake repos + fake ports (contract tests) |
| cli | test-after (wiring only) |
| e2e (mise.run, gh auth) | `//go:build e2e`, `make test-e2e` |
