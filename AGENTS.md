# Mison — AI Agent Guide

Reproduce development environments across machines. mise is the engine,
a GitHub repo holding `mise.toml` is the source of truth. Mison orchestrates.

**Read before touching**: `docs/DESIGN.md` (behavior specs) and
`docs/ARCHITECTURE.md` (layer rules). When modifying sync/merge policy
or command flows, you MUST read `docs/DESIGN.md` first.

## Commands

```bash
mise install              # set up dev env (go, golangci-lint) — dogfooding
make build                # build ./mison binary
make test                 # go test ./...
make test-e2e             # real-world tests (mise.run, gh auth)
make lint                 # golangci-lint run
make fmt                  # gofmt + go mod tidy
```

Run `make lint && make test` before finishing any task. Fix failures yourself.

## Architecture map (layered: cli → usecase → repo → service)

```
cmd/mison/            entrypoint (thin; version injected via ldflags)
internal/cli/         cobra wiring + TermUI adapter only — no logic
internal/usecase/     business flows: policy, error boundary, interaction ports
internal/repo/
  gitrepo/            atomic git + gh commands (recipes, no decisions)
  miserepo/           atomic mise commands (raw data, no filtering)
internal/service/     Runner: the ONLY place os/exec exists (process boundary)
internal/env/         mise.toml read/write/diff (pure domain — TDD core)
internal/detector/    OS/arch/mise detection (pure)
internal/xdg/         single source of XDG-aware paths
internal/paths/       on-disk layout (~/.mison/env, global-config symlink)
internal/ui/          low-level renderer (✓/↻/✗/⚠) used by TermUI
internal/e2e/         real-world tests, build tag `e2e`
```

### usecase layout (3 files)

```
internal/usecase/
├── ports.go       Reporter (notify) + Prompter (confirm) + ConflictPolicy
├── sync.go        PlanSync (pure), Engine (SmartPush/SmartPull/SyncStatus),
│                  ownership filter, conflict-side rules
└── commands.go    Flows: Install/Uninstall/Sync/Status/Init + error
                   classification (fatal vs warn-and-defer)
```

### cli layout (one command, one thin file)

```
internal/cli/
├── root.go        Execute wiring + flag helpers only
├── interact.go    TermUI (implements Reporter + Prompter on a terminal)
└── <command>.go   cobra shim per command (init, install, uninstall,
                   sync, status) — parse flags, call flows
```

## Layer access rules

```
✅ cli   → usecase
✅ usecase → repo/{gitrepo,miserepo}, service, env, paths, xdg, detector, ui
✅ repo/gitrepo → service
❌ service → anything above (it knows only env, name, args)
❌ usecase → service.Runner directly (must go through repos)
❌ repo/usecase → cli (ports only)
```

`service` is mison's process boundary — the only package that imports
`os/exec`. It is generic by design: one Runner for git, gh, mise, and sh.

## Interaction ports (business flows never touch io)

Flows reach the user exclusively through two ports on Flows:

- `UI Reporter` — one-way notifications (`Step/Synced/Warn/Fail/Line/ToolLine`)
- `Ask Prompter` — blocking confirmations (`Confirm`, `ResolveConflict`)

```
✅ Do    f.UI.Step("Installing node")       // notify
✅ Do    if !f.Ask.Confirm("Remove?") ...   // gate a destructive step
❌ Do not fmt.Fprint / fmt.Fprintf in flows or repos
❌ Do not read os.Stdin in flows — answers come only from Ask
```

Rationale: showing and asking are different concerns; the split makes
the "what is confirmed vs merely shown" policy explicit as types.
Tests inject fakeReport/fakeAsk and assert which port calls a flow
made, not rendered strings. Exception: gh device-flow login passes the
process stdio through (child-process UI, not ours) — via Runner.RunTTY.

## Design decisions (summary)

| Area | Decision |
|---|---|
| Language | Go + cobra |
| Declaration | `mise.toml` used directly (100% mise-compatible) |
| mise install | official installer (mise.run), never brew/apt |
| Auth | mise installs gh → device flow → `gh auth setup-git` |
| Commands | init, install, uninstall, sync, status (5 only) |
| install/uninstall | declare + apply + auto commit&push (fetch+rebase on reject) |
| sync | pull → apply (os-filtered) → push pending → orphan prompt |
| OS scoping | default all machines; `--mac`/`--linux[/x64|/arm64]` → mise `os` field |
| Repo | private env repo, owned by mison (auto-commit manual edits) |
| Layers | cli → usecase → repo → service; process boundary in service |

Full rationale: `docs/DESIGN.md`.

## Conventions

- Conventional Commits (`feat:`, `fix:`, `chore:`, `refactor:`, `test:`)
- Trunk-based: `main` + short-lived `feat/*` branches
- Errors: wrap with `fmt.Errorf("...: %w", err)`; service.Runner errors
  are self-describing (command + stderr) — don't re-add prefixes
- File name = primary type name (`runner.go` holds Runner); no comments
  unless non-obvious; table-driven tests, same-package test files

## TDD rules (pragmatic)

- **Pure logic** (`env`, `usecase.PlanSync`, ownership filter, `ui`,
  `xdg`, `detector`): strict RED-GREEN-REFACTOR. Failing test first.
- **Repos** (`gitrepo`, `miserepo`): fake Runner scripting for unit
  tests; the sync Engine is integration-tested with real git in
  `t.TempDir()` (usecase/sync_test.go).
- **Flows** (usecase commands): fake repos + fake ports (contract tests).
- **CLI surface** (cobra wiring): test-after is fine.
- e2e (mise.run install, gh auth): build tag `//go:build e2e`, run via
  `make test-e2e`.
- Coverage: pure packages ≥90%; no enforced global number.
- Each phase starts with a test list extracted from `docs/DESIGN.md`.

## Hard rules

- Never let the env repo enter a conflicted git state — semantic 3-way
  merge in Engine, never `git merge`/`git pull` (see docs/DESIGN.md).
- Never push without fetching first; on divergence: fetch → plan →
  semantic merge → push (PlanSync decides, Engine executes).
- Remote merges/conflict resolutions are ALWAYS shown to the user (↻).
- Only `internal/service` may import `os/exec`.
- Don't implement scan/adopt, Windows support, or provider abstraction —
  V1 scope is fixed.
- mise update policy: explicit only, never auto-update during sync.
