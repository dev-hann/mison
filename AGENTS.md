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
cmd/mison/          entrypoint (thin)
internal/cli/       cobra commands — no business logic here
internal/ui/        output rendering (✓/↻/✗/⚠)
internal/detector/  OS/arch/mise detection (pure)
internal/mise/      mise engine wrapper (Manager interface)
internal/env/       mise.toml read/write/diff (pure logic — TDD core)
internal/gitclient/ env repo git operations (M2)
```

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
