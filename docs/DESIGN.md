# Mison Design — Behavior Specifications

This document is the source of truth for runtime behavior. It is read-only
after approval: changes require a new decision entry. See PROJECT.md for the
original product brief. Command-by-command flows, edge cases, and their
guarding tests are mapped in BEHAVIOR.md.

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

### mison install <tools...>

```
1. mise install name@version per tool (one-off — mise never writes config)
2. only tools that installed locally are declared in mise.toml
3. regenerate mise.lock
4. commit "install: node, python" → push (fetch → semantic merge on rejection)
```

Apply-first (decision #17): a tool that fails to install — wrong name,
no version, offline, no local build — never enters the declaration.
Failures render as outcome classes (Applied/SkippedPlatform/Failed),
never half-declared state. Platform scoping comes from the lockfile's
per-tool platform keys (decision #10): a machine never attempts a
tool with no build for it. --mac/--linux flags do not exist.

Offline: the install attempt fails, so nothing is declared; retry online.

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

### mison update <tools...>

```
1. mise lock --global --bump --dry-run --json → candidates
   (fuzzy selectors only; exact pins untouched; mise.toml unmodified)
2. none → up-to-date noop; --dry-run stops after listing
3. confirm → mise lock --bump → mise install → push "update: old → new"
```

The explicit re-resolution path decision #12 implied — sync never
bumps. Only the lockfile advances and propagates to other machines.

### mison upgrade

```
1. refuse "dev" builds
2. GitHub API latest tag (plain HTTP, gh-independent)
3. newer → official install.sh (checksum-verified, keeps mison.old)
4. mise self-update (brew-managed mise → brew upgrade hint)
```

upgrade owns the BINARIES mison depends on; update owns declared
tools (decision #20).

### mison status

Read-only: stack header (mison·mise versions, floor guard), sync
state vs GitHub, per-tool ✓ installed / ✗ missing / ⚠ mismatch /
⊘ not-for-this-platform (lock-derived), path-backed warnings, and
mise doctor problems (noise-filtered — activation advice is always
false in mison's exec context).

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
| 6 | 7 commands (init/install/uninstall/sync/status/update/upgrade) | update+upgrade added in v0.5.0 (decisions #12/#20); scan/adopt, Windows still deferred |
| 7 | install/uninstall auto-push | user never runs git manually |
| 8 | fetch-before-push, rebase | stale-machine push is the normal path |
| 9 | semantic TOML merge | no conflicted state, key-level precision |
| 10 | union semantics + os field | one shared env, mise-native OS scoping |
| 11 | orphan prompt + --prune | safe default, scriptable override |
| 12 | explicit mise update only | sync never bumps; `mison update` is the explicit re-resolution path (mise lock --bump) |
| 13 | auto-commit manual edits | mison owns the repo; user edits are welcome |
| 14 | schema guard `[_.mison] schema` | forward-incompatibility shield (npm lockfileVersion pattern); older mison refuses newer schemas before any reset/push |
| 15 | repo name `mison-env`, persisted in ~/.mison/config.toml | short, self-describing; local persistence prevents default-drift split-brain across machines |
| 16 | no README in the env repo | mise.toml is the single source of truth; the repo is mison-owned
| 17 | install = apply-first | a tool is declared only after it installed locally — failures never pollute the shared declaration |
| 18 | sync applies per tool with outcome classes | Applied / SkippedPlatform / Failed — no mid-flow aborts; lock-derived platform filter skips tools with no local build |
| 19 | install failures exit non-zero; sync failures warn | registration denial must be visible; a pulled declaration is not this machine's to undeclare |
| 20 | `mison update` = tools, `mison upgrade` = binaries (mison + mise) | gh-style split; update is confirm-gated; upgrade reruns the checksum-verified installer and refuses "dev" builds |
| 21 | dual distribution: installer + homebrew tap | goreleaser pushes Formula/mison.rb to dev-hann/homebrew-tap on every tag (cross-repo PAT secret); caveat tells users to pick ONE method | gh-style split; update is confirm-gated (declaration versions stay fuzzy — only the lock advances); upgrade reruns the checksum-verified installer, refuses "dev" builds |

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

In: macOS/Linux, arm64/x86_64, the 7 commands (init/install/uninstall/
sync/status/update/upgrade), mise provider only.
Out: Windows, scan/adopt (scope recorded, deferred), provider
abstraction, secrets, dotfiles.
## 8. mise.lock adoption (design approved → implemented)

Implemented: `paths.Ensure` symlinks `~/.config/mise/mise.lock` →
`~/.mison/env/mise.lock`; install/uninstall refresh the lock before
their auto-push; sync/init regenerate after applying and push a
"mison: refresh lock" commit when content changed (registry failures
warn-and-defer, never blocking). Lock regeneration is skipped on
no-op syncs to avoid pointless registry round-trips.

Correction after live verification (mise 2026.9.1): `mise lock
--global` writes via atomic rename and REPLACES the symlink with a
regular file — the lock never reached the env repo through the link.
`Flows.refreshLock` therefore adopts the freshly written content into
the env repo and restores the symlink after every regeneration. The
env repo remains the single source of truth. Regeneration is
byte-deterministic across runs on identical state (e2e-verified), so
cross-machine convergence holds.

Live-researched against mise 2026.8 (`mise lock --global` on a real env):

**Facts observed**
- `mise lock --global` locks ALL 7 platforms (linux×2, musl×2, macos×2, windows) — cross-machine reproducibility built in.
- It pins fuzzy selectors to exact resolved versions (node "22" → 22.23.2) with sha256 + URLs.
- The lockfile lands at `~/.config/mise/mise.lock` — NEXT TO our config symlink, NOT through it. Without mison support it would never be synced.

**Design**
- Layout: repo holds `mise.lock`; `~/.config/mise/mise.lock` becomes a
  symlink → repo file (same pattern as config.toml; paths.Ensure gains
  a second symlink).
- Merge policy: mise.lock is DERIVED state — never semantic-merged.
  After any declaration change or merge, mison REGENERATES it
  (`mise lock --global`) and commits the result. Text conflicts during
  rebase are resolved by take-any-side + regenerate.
- Flow hooks: install/uninstall refresh the lock before their auto-push;
  sync regenerates after applying, then commits+pushes if it changed
  ("mison: refresh lock").
- Network: locking hits registries — best-effort with warn-and-defer
  (same policy as push), never blocks the flow.
- Schema: mise.lock is mise's own `@generated` format; mison's schema
  guard does not apply to it (mise owns its compatibility).

