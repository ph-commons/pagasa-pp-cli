#!/usr/bin/env bash
#
# pagasa-pp-cli fleet installer — idempotent, macOS + Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/ph-commons/pagasa-pp-cli/main/scripts/install.sh | bash
#
# Prefers the GitHub release prebuilt tarball (checksum-verified). Falls back
# to `go install` only when prebuilt cannot be resolved (no release asset,
# network/API failure, unsupported arch) — never after a checksum mismatch.
# If this machine has the ngpestelos fleet layout (~/src/hermes-config,
# ~/.claude/skills), it also wires the pp-pagasa skill; those steps are
# skipped cleanly elsewhere.
#
# Requires: curl (prebuilt path), and for source fallback Go 1.21+
# (GOTOOLCHAIN=auto fetches the toolchain the module needs). Checksum
# verify needs sha256sum (Linux) or shasum (macOS).

set -euo pipefail

MODULE="github.com/ph-commons/pagasa-pp-cli"
BIN="pagasa-pp-cli"
MCP="pagasa-pp-mcp"
GOBIN_DIR="${GOBIN:-$HOME/.local/bin}"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# SHA256 of a file (portable: GNU coreutils or macOS shasum).
file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "need sha256sum or shasum to verify release checksums"
  fi
}

OWNER_REPO="ph-commons/pagasa-pp-cli"
mkdir -p "$GOBIN_DIR"

# --- 1. Prefer the prebuilt release binary (no local compile) ----------------
# Compiling modernc.org/sqlite is CPU-heavy — never do it on a small VPS that's
# also running services. Download the CI-built binary instead; fall back to
# `go install` only if the download can't be resolved (not on hash mismatch).
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) arch="" ;;
esac

# True if path is a regular file (not a symlink). Avoid planting links under GOBIN.
is_regular_file() {
  [ -f "$1" ] && [ ! -L "$1" ]
}

install_ok=false
if [ -n "$arch" ] && command -v curl >/dev/null 2>&1; then
  # Digest tools are required for the prebuilt integrity path. Missing tools
  # is not an integrity failure — skip prebuilt and allow go install fallback.
  if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    warn "neither sha256sum nor shasum found; skipping prebuilt (will try go install)."
  else
    # Resolve the latest release tag (public repo, no auth needed).
    # || true so set -e/pipefail does not abort when sed/grep find no match
    # (API empty/offline) — that path should fall through to go install.
    ver="$(
      { curl -fsSL "https://api.github.com/repos/${OWNER_REPO}/releases/latest" 2>/dev/null \
        | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1; } || true
    )"
    if [ -n "$ver" ]; then
      tarball="pagasa-pp-cli_${ver}_${os}_${arch}.tar.gz"
      url="https://github.com/${OWNER_REPO}/releases/download/v${ver}/${tarball}"
      csum_url="https://github.com/${OWNER_REPO}/releases/download/v${ver}/checksums.txt"
      log "Downloading prebuilt $BIN v$ver ($os/$arch)"
      tmp="$(mktemp -d)"
      # shellcheck disable=SC2064
      trap 'rm -rf "$tmp"' EXIT

      if ! curl -fsSL "$csum_url" -o "$tmp/checksums.txt" 2>/dev/null; then
        warn "could not fetch checksums.txt for v$ver; will try building from source."
      elif ! curl -fsSL "$url" -o "$tmp/$tarball" 2>/dev/null; then
        warn "prebuilt download failed ($url); will try building from source."
      else
        # Field-equality match (not grep -E regex — dots in version are literal).
        # || true so a missing line does not trip pipefail before the die below.
        expected="$(
          awk -v f="$tarball" '$2 == f { print $1; exit }' "$tmp/checksums.txt" || true
        )"
        if [ -z "$expected" ]; then
          # Missing entry is an integrity gap — fail closed (do not extract).
          die "no checksum entry for ${tarball} in release checksums.txt"
        fi
        if ! printf '%s' "$expected" | grep -Eq '^[0-9a-fA-F]{64}$'; then
          die "malformed checksum for ${tarball} (want 64 hex digits, got: ${expected})"
        fi
        actual="$(file_sha256 "$tmp/$tarball")"
        if [ "$actual" != "$expected" ]; then
          die "checksum mismatch for ${tarball} (expected ${expected} got ${actual})"
        fi
        log "Checksum OK (${actual:0:12}…)"
        # Extract only expected members — refuse unexpected tar paths.
        if ! tar -xzf "$tmp/$tarball" -C "$GOBIN_DIR" "$BIN" "$MCP" 2>/dev/null; then
          die "tar extract failed for ${tarball} (expected members: $BIN $MCP)"
        fi
        if ! is_regular_file "$GOBIN_DIR/$BIN" || ! is_regular_file "$GOBIN_DIR/$MCP"; then
          die "extract missing regular-file members under $GOBIN_DIR (need $BIN + $MCP, not symlinks)"
        fi
        chmod +x "$GOBIN_DIR/$BIN" "$GOBIN_DIR/$MCP" 2>/dev/null || true
        install_ok=true
      fi

      rm -rf "$tmp"
      trap - EXIT
    fi
  fi
fi

# --- 2. Fallback: build from source (go install) -----------------------------
if [ "$install_ok" != true ]; then
  command -v go >/dev/null 2>&1 || die "No prebuilt binary available and Go not on PATH. Install Go 1.21+ (https://go.dev/dl/) or check the release assets."
  warn "Building from source — this compiles modernc.org/sqlite and is CPU-heavy."
  # A brand-new public module can 500 on sum.golang.org while the checksum DB
  # indexes it; retry a few times before giving up.
  install_err="$(mktemp "${TMPDIR:-/tmp}/pagasa-install.XXXXXX.err")"
  for attempt in 1 2 3; do
    if GOTOOLCHAIN=auto GOBIN="$GOBIN_DIR" go install \
      "${MODULE}/cmd/${BIN}@latest" \
      "${MODULE}/cmd/${MCP}@latest" \
      2>"$install_err"; then
      install_ok=true
      break
    fi
    if grep -q "sum.golang.org" "$install_err" 2>/dev/null; then
      warn "checksum DB not ready (attempt $attempt/3); retrying in 10s"
      sleep 10
    else
      cat "$install_err" >&2
      break
    fi
  done
  rm -f "$install_err"
fi
[ "$install_ok" = true ] || die "install failed (neither prebuilt download nor go install worked)."

# --- 3. Verify ---------------------------------------------------------------
BIN_PATH="$GOBIN_DIR/$BIN"
MCP_PATH="$GOBIN_DIR/$MCP"
[ -x "$BIN_PATH" ] || die "binary not found at $BIN_PATH after install."
if [ ! -x "$MCP_PATH" ]; then
  warn "MCP binary missing at $MCP_PATH (CLI installed; install MCP separately if needed)."
fi
log "Installed: $("$BIN_PATH" --version 2>/dev/null || echo "$BIN (version unknown)")"
"$BIN_PATH" now --dry-run >/dev/null 2>&1 && log "Smoke check passed (now --dry-run)." || warn "dry-run smoke check failed; the binary installed but check PATH/network."

case ":$PATH:" in
  *":$GOBIN_DIR:"*) : ;;
  *) warn "$GOBIN_DIR is not on your PATH — add it so agents can find $BIN." ;;
esac

# --- 4. Fleet skill wiring (guarded; no-op off the fleet) --------------------
HERMES_CONFIG="$HOME/src/hermes-config"
if [ -d "$HERMES_CONFIG/skills/pp-pagasa" ]; then
  log "Refreshing hermes-config (pp-pagasa skill)"
  git -C "$HERMES_CONFIG" pull --rebase --autostash -q 2>/dev/null || warn "could not pull hermes-config; skill may be stale."
  # Claude Code discovers skills via ~/.claude/skills symlinks; Hermes reads
  # hermes-config directly and needs no symlink.
  if [ -d "$HOME/.claude/skills" ]; then
    ln -sfn "$HERMES_CONFIG/skills/pp-pagasa" "$HOME/.claude/skills/pp-pagasa"
    log "Linked pp-pagasa into ~/.claude/skills (Claude Code)."
  fi
  log "pp-pagasa skill ready. Restart the Hermes gateway/session to load it."
else
  log "No fleet skill layout here — binary only. (pp-pagasa lives in ~/src/hermes-config on the fleet.)"
fi

log "Done."
