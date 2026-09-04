#!/bin/sh
# mison installer — downloads a release binary into ~/.local/bin.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/dev-hann/mison/main/scripts/install.sh | sh
#
# Pin a version:
#   MISON_VERSION=v0.2.1 curl -fsSL ... | sh
#
set -eu

REPO="dev-hann/mison"
VERSION="${MISON_VERSION:-}"

log()  { printf 'mison: %s\n' "$1"; }
fail() { printf 'mison: %s\n' "$1" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux" ;;
  *) fail "unsupported OS: $(uname -s) (mison supports macOS and Linux)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ -z "$VERSION" ]; then
  log "looking up the latest release"
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$VERSION" ] || fail "could not resolve the latest release"
fi

log "installing mison ${VERSION} for ${OS}/${ARCH}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

BASE="https://github.com/${REPO}/releases/download/${VERSION}"
TARBALL="mison_${OS}_${ARCH}.tar.gz"

log "downloading ${TARBALL}"
curl -fsSL -o "${TMP}/${TARBALL}" "${BASE}/${TARBALL}"

log "verifying checksum"
curl -fsSL -o "${TMP}/checksums.txt" "${BASE}/checksums.txt"
WANT="$(sed -n "s/ *\([0-9a-f]\{64\}\)  ${TARBALL}\$/\1/p" "${TMP}/checksums.txt")"
[ -n "$WANT" ] || fail "checksum for ${TARBALL} not found in checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  GOT="$(sha256sum "${TMP}/${TARBALL}" | cut -d' ' -f1)"
elif command -v shasum >/dev/null 2>&1; then
  GOT="$(shasum -a 256 "${TMP}/${TARBALL}" | cut -d' ' -f1)"
else
  fail "neither sha256sum nor shasum is available — cannot verify the download"
fi
[ "$GOT" = "$WANT" ] || fail "checksum mismatch (want $WANT, got $GOT) — aborting"

BIN_DIR="${HOME}/.local/bin"
mkdir -p "$BIN_DIR"

if [ -f "${BIN_DIR}/mison" ]; then
  cp "${BIN_DIR}/mison" "${BIN_DIR}/mison.old"
  log "existing binary backed up to mison.old"
fi

tar -xzf "${TMP}/${TARBALL}" -C "$BIN_DIR" mison
chmod +x "${BIN_DIR}/mison"

log "installed to ${BIN_DIR}/mison"

# --- shell wiring (fnm pattern, grep-idempotent) ----------------------
# Adds ~/.local/bin to PATH in the user's shell rc. Opt out with
# --skip-shell or MISON_SKIP_SHELL=1; skipped automatically in CI.
# (mise activation is NOT wired here — mise does not exist until
# `mison init` installs it; init adds that line itself.)
MISON_SKIP_SHELL="${MISON_SKIP_SHELL:-}"
[ "${1-}" = "--skip-shell" ] && MISON_SKIP_SHELL=1
[ -n "${CI-}" ] && MISON_SKIP_SHELL=1

if [ -z "$MISON_SKIP_SHELL" ]; then
  RC=""
  case "$(basename "${SHELL:-}")" in
    zsh)  RC="${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash) if [ "$(uname -s)" = "Darwin" ]; then RC="$HOME/.bash_profile"; else RC="$HOME/.bashrc"; fi ;;
  esac
  if [ -z "$RC" ]; then
    log "note: add ${BIN_DIR} to your PATH manually (unsupported shell: ${SHELL:-unknown})"
  elif grep -q '\.local/bin' "$RC" 2>/dev/null; then
    log "PATH already wired in ${RC}"
  else
    mkdir -p "$(dirname "$RC")"
    printf '\n# mison (installer)\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$RC"
    log "added ~/.local/bin to PATH in ${RC} — open a new terminal to use mison"
  fi
fi

log "next: mison init   (installs mise, links GitHub, activates mise in your shell)"
