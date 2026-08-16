# Mison — AI Agent Guide

Reproduce development environments across machines. mise is the engine,
a GitHub repo holding `mise.toml` is the source of truth. Mison orchestrates.

**Read before touching**: `docs/DESIGN.md` (behavior specs) and
`docs/ARCHITECTURE.md` (package rules). When modifying `internal/gitclient/`,
sync pipeline, or conflict handling, you MUST read `docs/DESIGN.md` first.

## Commands

```bash
mise install              # set up dev env (go, golangci-lint) — dogfooding
make build                # build ./mison binary
make test                 # go test ./...
make test-coverage        # coverage report
make lint                 # golangci-lint run
make fmt                  # gofmt + go mod tidy
```

Run `make lint && make test` before finishing any task. Fix failures yourself.

## Architecture map

```
cmd/mison/          entrypoint (thin; version injected via ldflags)
internal/cli/       cobra commands — no business logic here
internal/ui/        output rendering (✓/↻/✗/⚠)
internal/detector/  OS/arch/mise detection (pure)
internal/xdg/       single source of XDG-aware paths (config/data, mise shims)
internal/mise/      mise engine wrapper (Manager interface)
internal/env/       mise.toml read/write/diff (pure logic — TDD core)
internal/gitclient/ env repo git operations (semantic merge policy)
internal/gh/        gh CLI wrapper (auth, repo create)
internal/e2e/       real-world tests, build tag `e2e`
```

### cli file layout (one command, one file)

```
internal/cli/
├── app.go         App struct + shared helpers only (ui/layout/config IO)
├── root.go        cobra wiring only
├── interact.go    interaction ports: Reporter (notify) + Prompter (confirm)
├── <command>.go   one command handler per file (install, uninstall,
│                  sync, status, init) plus its private helpers
└── git_hook.go    shared push policy (commitAndPush, conflict resolver)
```

Rules: App struct fields live only in app.go; a new command gets its
own file; cross-command helpers go in app.go or git_hook.go.

### Interaction ports (business flows never touch io)

Flows reach the user exclusively through two ports on App:

- `UI Reporter` — one-way notifications (`Step/Synced/Warn/Fail/Line/ToolLine`)
- `Ask Prompter` — blocking confirmations (`Confirm`, `ResolveConflict`)

```
✅ Do    a.UI.Step("Installing node")      // notify
✅ Do    if !a.Ask.Confirm("Remove?") ...  // gate a destructive step
❌ Do not fmt.Fprint / fmt.Fprintf in command handlers
❌ Do not read os.Stdin in flows — answers come only from Ask
```

Rationale: showing and asking are different concerns; the split makes
the "what is confirmed vs merely shown" policy explicit as types.
Tests inject fakeReport/fakeAsk and assert which port calls a flow
made, not rendered strings. Exception: gh device-flow login passes the
process stdio through (child-process UI, not ours).

Dependency direction: cli → {detector, mise, env, gitclient, ui}.
Pure packages (`env`, `detector`, `ui`) must not import exec/os machinery.

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

Full rationale: `docs/DESIGN.md`.

## Conventions

- Conventional Commits (`feat:`, `fix:`, `chore:`, `refactor:`, `test:`)
- Trunk-based: `main` + short-lived `feat/*` branches
- Errors: wrap with `fmt.Errorf("...: %w", err)`; user-facing messages only in `internal/ui`
- No comments in code unless non-obvious; no `else` after return when avoidable
- Table-driven tests, same-package test files

## TDD rules (pragmatic)

- **Pure logic** (`env`, `detector`, `ui`, merge/diff): strict RED-GREEN-REFACTOR.
  Write the failing test first. Never write implementation before its test.
- **I/O shells** (`mise`, `gitclient`): mock via interfaces for unit tests;
  integration tests run real git in `t.TempDir()`.
- **CLI surface** (cobra wiring): test-after is fine.
- e2e (mise.run install, gh auth): build tag `//go:build e2e`, CI-only.
- Coverage: pure packages ≥90%; no enforced global number.
- Each phase starts with a test list extracted from `docs/DESIGN.md`.

## Hard rules

- Never let the env repo enter a conflicted git state — semantic 3-way merge
  in code, abort rebase on conflict, prompt the user (see docs/DESIGN.md).
- Never push without fetching first; on rejection: fetch → rebase → re-push.
- Remote merges/conflict resolutions are ALWAYS shown to the user (↻ line).
- Don't implement scan/adopt, Windows support, or provider abstraction — V1 scope is fixed.
- mise update policy: explicit only, never auto-update during sync.
