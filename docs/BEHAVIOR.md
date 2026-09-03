# Mison Behavior Reference — Command Flows, Edge Cases, Test Map

Living reference generated from the codebase (v0.2.x). Use it to
answer "what happens when X?" without reading code. When behavior
changes, update this file in the same commit.

Test names refer to `internal/usecase/{sync,commands}_test.go` and
`internal/env/*_test.go` unless noted.

## Common preconditions (every command)

```
mise missing          → auto-install via mise.run      TestRunInstallInstallsMiseWhenMissing
mise.toml schema > 1  → hard error "upgrade mison"     TestParseRejectsFutureSchema
                                                        TestSmartPullRejectsFutureSchema*
~/.mison/env + config symlink ensured (idempotent)     TestEnsureCreatesEnvDirAndFile
                                                        TestEnsureIdempotent
~/.config/mise/mise.lock symlink → env repo lockfile   TestEnsureSymlinksGlobalLock
```

## mison init

```
① detect OS/arch → report
② paths.Ensure (env dir + mise.toml + symlink)
③ ensureMise (install if missing)
④ ensureGh:
   gh missing → mise install gh@latest        [bootstrap chain]
   declare gh in mise.toml automatically      [every machine self-bootstraps]
   unauthenticated → device-flow login (stdio passthrough)
   gh auth setup-git (git credentials)
⑤ connectRepo — three cases:
   a. local clone already connected → SmartPull
   b. remote repo exists (another machine made it) → Connect (fetch+reset)
      └ remote empty → seed push
   c. neither → create private repo → init → seed push
⑥ persist repo name to ~/.mison/config.toml   [flag > persisted > default]
⑦ mise install (apply declaration)
```

| Edge case | Behavior | Test |
|---|---|---|
| gh entirely missing | installed via mise, declared, authenticated — full chain | TestRunInitGhNotInstalledFlow |
| second machine (repo exists) | Connect instead of create | TestRunInitSecondMachineConnectsExistingRepo |
| create race (repo appears between check and create) | create fails → re-check → Connect | TestRunInitCreateRaceFallsBackToConnect |
| remote repo exists but empty | Connect then seed push | TestRunInitExistingEmptyRepoSeedsInitialPush |
| local clone exists, init re-run | SmartPull only | TestRunInitConnectsExistingRepo |
| README expectations | none written — mise.toml is the whole repo (decision #16) | TestRunInitDoesNotWriteReadme |
| default repo name drifts across versions | persisted name wins → no split-brain | TestRunInitPersistsRepoNameLocally |
| explicit --repo flag vs persisted name | flag wins and is re-persisted | TestRunInitExplicitFlagWinsOverPersisted |
| machine without git identity | repo-local mison@local fallback | TestSmartPushWithoutGitIdentity |
| Connect target dir already has files | init+fetch+reset (not clone) | TestConnectFreshDir |
| remote uses future schema | Connect refuses before reset; worktree untouched | TestConnectRejectsFutureSchemaRemote |

## mison install <tools...> [--mac/--linux] [--ours/--theirs]

```
① parse specs (name@version, os flags)
② edit declaration: SetTool (merges, preserves options)
③ saveConfig → schema stamp
④ OS-restricted tool not for this machine → ⚠ skip notice
⑤ mise install
⑥ visibility re-check (silent no-op detection → ⚠)
⑦ commitAndPush: "install: X, Y"
```

| Edge case | Behavior | Test |
|---|---|---|
| malformed spec (`node@`) | immediate error, nothing changed | TestRunInstallInvalidSpec |
| OS restriction doesn't match machine | declared, install skipped + notice | TestRunInstallWarnsOSSkip |
| mise install silent no-op (broken symlink) | post-check warns | TestRunInstallWarnsStillMissing |
| push fails (offline) | warn-and-defer to next sync, command succeeds | TestInstallDeferredPushOnFailure |
| future-schema remote at push | fatal: Fail port, non-zero exit (no defer) | TestInstallFutureSchemaPushIsFatal |
| other machine's changes found at push | auto-merge + mandatory ↻ notice | TestInstallShowsRemoteMergeNotice |
| existing tool options (postinstall...) | preserved on version/os change | TestSetToolPreservesExistingOptions, TestSetToolAddsOSWhileKeepingOptions, TestSetToolRemovesOnlyOS |
| multiple tools at once | one commit + one push | TestRunInstallWritesDeclarationAndApplies |
| lockfile refresh after apply | `mise lock --global` before push — same commit; failure warns + defers | TestInstallRefreshesLockBeforePush, TestInstallLockFailureWarnsAndDefers |
| first machine (no repo connected) | push silently skipped (local-only mode) | TestRunInstallCreatesSymlink (repo-less path) |

## mison uninstall <tools...> [--yes]

```
① confirm (y/N) — decline ends here
② remove from declaration (unknown tool → error)
③ installed locally → mise uninstall --all
④ commitAndPush: "uninstall: X"
```

| Edge case | Behavior | Test |
|---|---|---|
| user declines | NO changes of any kind (gate before everything) | TestRunUninstallAbortsWhenDeclined, TestUninstallFlowDeclinedStopsBeforeAnyChange |
| tool not in declaration | error "not in environment" | TestRunUninstallUnknownTool |
| installed elsewhere but not here | declaration removed + "not installed" notice | TestRunUninstallRemovesDeclarationAndTool |
| confirm→remove→push ordering | mise runs and push happens only after confirmation | TestUninstallFlowAsksConfirmationBeforeRemoving |

## mison sync [--prune] [--ours/--theirs]

```
① no mise.toml → "run mison init first"
② restore config symlink (clone-only machines)
③ repo connected → SmartPull (engine below)
④ declaration vs installed diff → mise install
⑤ visibility re-check
⑥ orphans (installed but undeclared) → prompt / --prune / keep
⑦ report
```

Engine `sync()` — the shared path for ALL pushes/pulls:

```
uncommitted manual edits → auto-commit ("mison: manual changes")
fetch
resolve head / remote / base
PlanSync four-way:
  SeedRemote  → push to seed
  Push        → push pending commits
  FastForward → parse remote FIRST → reset --hard
  Merge       → 3-way over base/local/remote:
                  different tools → auto-merge (unattended)
                  same tool both changed → Resolver (prompt / --ours / --theirs)
                → reset to remote → rebuild mise.toml with merged tools
                → commit → push
```

| Edge case | Behavior | Test |
|---|---|---|
| no environment | init hint | TestRunSyncWithoutEnvironment |
| manual uncommitted edits + pull (Case F) | edits auto-committed, preserved AND pushed | TestSmartPullPreservesManualEdits |
| already synchronized | no-op "Already synchronized" | TestRunSyncNoopWhenAligned, TestSmartPullUpToDate |
| concurrent installs, different tools (diverged) | auto-merge, unattended, ↻ notice | TestSmartPushDivergedAutoMerge, TestSmartPullDivergedWithLocalPending |
| same tool, both changed differently | prompt [1/2] or --ours/--theirs | TestSmartPushConflictResolvedLocal/Remote, TestConflictResolutionRoutesThroughPrompter |
| removal vs edit conflict | promoted to conflict — no silent destruction | TestMergeRemovalVsChangeConflict (env layer) |
| offline deferred commits (ahead) | sync pushes pending after pull | TestSyncPullsBeforeApply |
| future-schema remote | rejected BEFORE any reset/push; worktree untouched | TestSmartPullRejectsFutureSchemaWithoutTouchingWorktree, TestSmartPushRefusesToPushOntoFutureSchemaRemote |
| pull network failure | warn, continue with local declaration | RunSync warn-and-continue (observed in live e2e) |
| future-schema remote at pull | fatal: hard error, sync aborted | TestSyncFutureSchemaPullIsFatal |
| orphans present | prompt → remove on approval / --prune unattended | TestRunSyncPruneRemovesOrphans |
| gh undeclared but installed | never offered as orphan (bootstrap protection) | TestSyncNeverPrunesGh |
| prune failure mid-list | remaining orphans still attempted; partial failure reported | TestSyncPruneContinuesPastFailures |
| orphan removal declined | "kept" notice + --prune hint | (symmetric path of the prune test) |
| machine missing the symlink (clone-only) | sync restores it via Ensure | TestRunSyncRestoresGlobalSymlink |
| lockfile after apply | regenerated; changed content → "mison: refresh lock" push; no-op sync skips regen | TestSyncPushesRefreshedLock, TestSyncSkipsLockPushWhenUnchanged |
| remote lock checkout | FastForward reset brings remote's mise.lock into the worktree | TestSmartPullFastForwardChecksOutLock |
| nothing to commit | clean tree → commit skipped | TestSmartPushSkipsEmptyCommit |
| partially applied state (failed prior sync) | always re-diff → reinstall (idempotent repair) | TestRunSyncAppliesMissing, TestRunSyncWarnsStillMissing |
| unrelated histories (manually seeded repo) | base="" → merge path | TestPlanSyncTable row |

## mison status (read-only)

```
① parse declaration ② installed list (ownership-filtered) ③ diff
④ sync state: fetch + four-way + tool diff (no mutation)
⑤ render: ✓/✗/⚠ + sync section
```

| Edge case | Behavior | Test |
|---|---|---|
| mixed states | ✓ installed / ✗ missing (sync hint) / ⚠ mismatch | TestRunStatusRendersStates |
| remote ahead | "↻ remote has new tools (x, y) — run mison sync" | TestSyncStatusBehind |
| local unpushed | ⚠ "local changes not pushed" | TestSyncStatusAhead |
| both changed | ⚠ "diverged" | TestSyncStatusDiverged |
| repo not connected | informational line, not an error | renderSyncStatus path |
| non-mison tools (project mise.toml owned) | excluded by ownership filter — no false positives | TestOwnedToolsFiltersBySourceAndActivity, TestOwnedToolsMatchesDeclarationPathDirectly |
| remote compare fails (offline) | warning, local status still printed | observed in live e2e (fetch 128) |

## Domain-layer fine edges (env — foundation of everything)

| Case | Rule | Test |
|---|---|---|
| `"22"` vs installed `22.11.0` | prefix match = equal | TestDiffPrefixMatch |
| `latest` declared | never string-compared (mise's domain) | TestDiffLatestNeverStringMismatch |
| unknown sections ([env]/[tasks]) | preserved on rewrite | TestBytesPreservesUnknownSections |
| os flag conversion (`--mac=arm64`) | normalized to `macos/arm64`; invalid → nil | TestParseOSSpec, TestParseOSSpecInvalid, TestAppliesTo |
| OS-restricted tools in merge | os arrays are 3-way merged too | TestMergeOSEntryChange, TestMergeOSConflict |
| option-only edit (postinstall, one side) | taken from the changed side — never silently dropped | TestMergeOptionOnlyRemoteEditTaken |
| option-only edit, both sides differ | promoted to conflict | TestMergeOptionBothChangedDifferentlyConflicts |
| merge winner applied to entry | non-nil Options = exact replace; nil = preserve existing | TestSetToolExactOptionsReplace, TestSetToolNilOptionsPreservesExisting |

## Known gaps (honest list — candidates, not commitments)

| Gap | Status |
|---|---|
| fetch↔push race (another machine pushes in between) | push rejected → next sync self-heals (structurally safe); no dedicated test |
| corrupted/old mise binary | undetected; `mise doctor` integration is a future candidate |
| damaged .git directory | error propagates; no recovery guidance |
| very large declarations (hundreds of tools) | unmeasured; currently linear |
| explicit test for the orphan-declined path | implied by symmetry with prune test; worth adding |
