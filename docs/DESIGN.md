# Mison Design — Behavior Specifications

This document is the source of truth for runtime behavior. It is read-only
after approval: changes require a new decision entry. See PROJECT.md for the
original product brief.

## 1. Model

Three states exist:

```
① Remote declaration — mise.toml on the GitHub env repo (source of truth)
② Local declaration   — mise.toml in the local clone (~/.mison/env/)
③ Local reality       — tools actually installed via mise
```

- `install`/`uninstall` mutate ② and ③, then commit & push (① follows).
- `sync` pulls ① into ②, then makes ③ match ② (os-filtered).

Union semantics: any machine's install adds to the shared declaration.
OS-specific tools use mise's native `os` field, not machine-specific files.

## 2. Commands

### mison init

```
1. detect OS/arch        5. gh auth login (device flow)
2. install mise          6. gh auth setup-git
3. mise install gh       7. create/connect private env repo
4. add gh to mise.toml   8. clone to ~/.mison/env/, symlink
                          ~/.config/mise/config.toml → clone
```

### mison install \<tools...\> [--mac | --linux[/x64|/arm64]]

```
1. update mise.toml [tools] (add os field if flag given)
2. mise install (apply locally)
3. commit "install: node, python"
4. push — on rejection: fetch → rebase → (semantic merge) → re-push
```

Offline: local apply + commit succeed; push deferred to next sync (warn).

### mison uninstall \<tools...\> [--yes]

Same pipeline, removal. Multi-tool confirmation prompt once (skip with --yes).

### mison sync

```
0. ensure mise/gh (bootstrap chain if missing)
1. pull --rebase (semantic merge on same-key conflicts → prompt)
2. push anything pending (deferred commits, merge results)
3. mise install: apply declaration, os-restricted tools auto-skipped by mise
4. orphan detection: installed-but-not-declared → prompt → remove
   (--prune: auto-remove, --no-prune: skip)
5. report
```

Sync is idempotent and doubles as a repair command (missing tools reinstall).

### mison status

Read-only diff of ② vs ③: ✓ installed / ✗ missing / ⚠ version mismatch.

## 3. Push conflict resolution

mise.toml `[tools]` is a flat table. Different keys auto-merge.

| Case | Situation | Handling |
|---|---|---|
| A | Stale local, different tool installed remotely | push rejected → fetch → rebase (auto) → push. Zero interaction |
| B | Same tool, one side changed | rebase auto-picks the change |
| C | Same tool, both changed differently | semantic 3-way merge detects key conflict → prompt: [1] keep local [2] accept remote [3] abort (leave unpushed). Non-interactive: `--ours`/`--theirs` |

Semantic merge: parse base/local/remote TOML → per-tool 3-way compare →
collect conflicting keys only. The repo never enters a git conflicted state;
rebase is aborted before file conflicts are written.

## 4. Notification rules

Principle: hide complexity (git), never hide information (changes).

| Event | Output |
|---|---|
| Local step | `✓ <step>` |
| Remote merge happened | `↻ Remote had new changes (<tools>) — merged automatically` — ALWAYS shown |
| Conflict | `✗` + interactive prompt |
| Non-interactive (CI) | ↻ logged to stderr; conflicts fail fast with flag hint |

## 5. Decisions log

| # | Decision | Rationale |
|---|---|---|
| 1 | Go + cobra | single binary, cross-compile, stdlib-rich CLI |
| 2 | mise.toml directly | 100% mise compat; users can use mise itself |
| 3 | mise via mise.run | one path for macOS+Linux, no OS package logic |
| 4 | gh via mise + device flow | no PAT storage/OAuth impl; gh owns tokens |
| 5 | gh declared in mise.toml | env bootstraps itself on any machine |
| 6 | 5 commands only | Minimal V1; scan/adopt deferred |
| 7 | install/uninstall auto-push | user never runs git manually |
| 8 | fetch-before-push, rebase | stale-machine push is the normal path |
| 9 | semantic TOML merge | no conflicted state, key-level precision |
| 10 | union semantics + os field | one shared env, mise-native OS scoping |
| 11 | orphan prompt + --prune | safe default, scriptable override |
| 12 | explicit mise update only | sync stays fast and predictable |
| 13 | auto-commit manual edits | mison owns the repo; user edits are welcome |

## 6. Sync case matrix

| Case | State | Behavior |
|---|---|---|
| New machine | no clone | requires `init` first |
| Up to date | ①=②=③ | no-op, "Already synchronized" |
| Remote ahead | other machine installed/uninstalled | pull → install new / orphan-prompt removed |
| Local pending | offline install deferred push | pull → push pending → no-op apply |
| Diverged | both sides changed | semantic merge → prompt if same key |
| Manual edits | uncommitted mise.toml changes | auto-commit, then diverged path |
| Partial install | last sync failed mid-way | always re-diff ③, reinstall missing |

## 7. V1 scope (fixed)

In: macOS/Linux, arm64/x86_64, the 5 commands, mise provider only.
Out: Windows, scan/adopt, provider abstraction, secrets, dotfiles.
