# mison

> Reproduce your development environment anywhere.

`mison` keeps development environments in sync across machines — macOS or
Linux, Apple Silicon or x86_64. Declare your tools once, push to GitHub,
and every other machine becomes identical with a single command.

```
Machine A (macOS)                Machine B (Linux)
$ mison init                     $ mison init
$ mison install node@22 rg       $ mison sync

        GitHub (private env repo)
              mise.toml
   node = "22" · rg = "latest" · gh = "latest"
```

## How it works

mison is a thin orchestration layer over proven tools — it is **not** a
package manager:

- **[mise](https://mise.jdx.dev)** is the installation engine. mison never
  implements per-OS install logic; mise resolves the right build for the
  current OS/architecture, including tools restricted via mise's native
  `os` field.
- **GitHub** is the source of truth. Your declarations live in a private
  repository (`mison-env`) holding a single `mise.toml` — nothing else.
- **[gh](https://cli.github.com)** handles auth. mison installs gh *through
  mise* (so gh is itself part of your synced environment) and uses device-flow
  login plus git credential setup. No tokens stored by mison.

The declaration is a plain mise config — you can use mise directly against
it at any time.

## Install

Requirements: `git`, `curl`. Everything else (mise, gh, tools) is
bootstrapped automatically.

```bash
curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh
```

Or pin a version:

```bash
MISON_VERSION=v0.2.1 curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh
```

The installer detects your OS/architecture, downloads the release
binary, and verifies its checksum. Prefer reviewing first?

```bash
curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh -o install.sh
less install.sh && sh install.sh
```

## Usage

```bash
mison init                    # bootstrap: mise → gh → auth → private env repo
mison install node@22 rg      # declare + install + auto commit & push
mison install docker --linux  # OS-scoped: only installs on Linux machines
mison uninstall node --yes    # remove everywhere + auto push
mison sync                    # pull latest declaration and apply it
mison status                  # compare declaration vs installed (✓ ✗ ⚠)
```

### Sync semantics

- Any machine's install adds to the **shared environment** (union merge).
- Concurrent installs on different machines auto-merge — different tools
  merge silently, same tool with different versions prompts
  (`--ours` / `--theirs` for non-interactive runs).
- Offline installs are committed locally and pushed on the next sync.
- Tools that vanished from the declaration are detected as orphans and
  removed after a prompt (`--prune` / answer `y`).

## Design

See [docs/DESIGN.md](docs/DESIGN.md) for behavior specifications (sync
case matrix, conflict policy, notification rules) and
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for package structure.

Scope (V1): macOS/Linux, the five commands above, mise as the only
provider. Explicitly out: Windows, dotfiles (use chezmoi), secrets.

## Development

```bash
mise install        # go, golangci-lint (dogfooding)
make test           # unit + git integration tests
make lint
```

Built test-first; pure logic (TOML merge/diff) is kept separate from
I/O shells. MIT license.
