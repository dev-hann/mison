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

**Homebrew:**

```bash
brew tap dev-hann/homebrew-tap
brew install mison
```

**Or the standalone installer** — Requirements: `git`, `curl`.
Everything else (mise, gh, tools) is bootstrapped automatically.

```bash
curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh
```

Or pin a version:

```bash
MISON_VERSION=v0.5.5 curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh
```

The installer detects your OS/architecture, downloads the release
binary, verifies its checksum, and adds `~/.local/bin` to your shell
rc automatically (opt out with `--skip-shell`; skipped in CI). mise
activation is wired later by `mison init`.

Optional — shell completion:

```bash
# zsh
mison completion zsh > "${fpath[1]}/_mison"
```

Prefer reviewing first?

```bash
curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh -o install.sh
less install.sh && sh install.sh
```

## Usage

```bash
mison init [--account <login>] # bootstrap: mise → gh → env repo (pins the account)
mison install node@22 rg      # install first; success earns a declaration + push
mison uninstall node --yes    # remove everywhere + auto push
mison sync                    # pull latest declaration and apply it
mison status                  # full health view: versions, sync state, tools, mise doctor
mison update [--dry-run]      # re-resolve fuzzy versions (latest, "22") + install
mison upgrade                 # upgrade binaries: mison + mise
```

Platform scoping is automatic: mison reads each tool's supported
platforms from the synced lockfile and never attempts a tool with no
build for the current machine (shown as "not for this platform").

After `mison init` on a fresh machine, run `exec zsh` (or open a new
terminal) — the init command wires `mise activate` into your shell rc
automatically, but a running terminal can only pick that up by
restarting. `mison init --no-shell-setup` leaves rc files untouched.

### Sync semantics

- Any machine's install adds to the **shared environment** (union merge).
- Concurrent installs on different machines auto-merge — different tools
  merge silently, same tool with different versions prompts
  (`--ours` / `--theirs` for non-interactive runs).
- Offline installs are committed locally and pushed on the next sync.
- Tools that vanished from the declaration are detected as orphans and
  removed after a prompt (`--prune` / answer `y`).

## Limitations

- **GitHub.com only** — GitHub Enterprise is not supported (auth and
  repo URLs hardcode `github.com`).
- **One environment per machine** — the local binding
  (`~/.mison/config.toml`) holds a single env repo; juggling work and
  personal environments means re-binding with `mison init --repo`.
- **Non-portable tools** — `path:` and other local-backed tools
  reference machine-local paths; mison warns whenever the declaration
  contains one, and they fail on machines where the path doesn't exist
  until uninstalled.
- **No concurrent runs on one machine** — a second concurrent mison
  command refuses to start (run mutex). Run them sequentially.
- **Air-gapped machines** — first setup and seeding need network
  access; afterwards offline deferral applies.

## Using mise directly

mison orchestrates the SHARED declaration (mise.toml, mise.lock, the
env repo). Everything session- or project-local stays mise's job —
mise is on your PATH and fully compatible:

- run tasks: `mise run <task>`
- one-off command envs: `mise exec node@22 -- node -v`
- shell session versions: `mise shell node@22`
- search the registry: `mise search ripgrep`
- config editing/formatting: `mise edit`, `mise fmt`

If a command mutates the global config (e.g. `mise use -g`), mison
picks the change up on the next sync and commits it.

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
