#!/usr/bin/env bash
# lazyport installer for Linux and macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/papasaidfine/lazyport/main/install.sh | bash
#
# Environment overrides:
#   VERSION       — release tag to install (default: latest, including prereleases)
#   INSTALL_DIR   — where to drop the binary (default: /usr/local/bin if
#                   writable, otherwise $HOME/.local/bin)
#
# What it does:
#   1. detect OS + arch
#   2. resolve target version
#   3. download tarball + checksums.txt
#   4. verify SHA-256 against checksums.txt
#   5. extract and install the binary

set -euo pipefail

REPO="papasaidfine/lazyport"
BIN="lazyport"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-}"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxxx\033[0m %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "need '$1' on PATH"; }

http_get() {
  # http_get URL OUTFILE
  local url="$1" out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$out"
  else
    die "need curl or wget"
  fi
}

http_get_stdout() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$1" -O -
  else
    die "need curl or wget"
  fi
}

detect_os() {
  case "$(uname -s)" in
    Linux)  echo linux ;;
    Darwin) echo darwin ;;
    *) die "unsupported OS: $(uname -s) — try the Windows installer or build from source" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) die "unsupported arch: $(uname -m)" ;;
  esac
}

resolve_version() {
  local v="$1"
  if [[ "$v" != "latest" ]]; then
    echo "$v"
    return
  fi
  # Pull the most recent release (including prereleases). The /releases/latest
  # endpoint excludes prereleases, so we use /releases and pick the newest by
  # created_at. We do NOT trust the API's array order — GitHub returns hits
  # sorted lexically by tag_name, which puts "v0.1.0-beta.9" above
  # "v0.1.0-beta.11". The awk script below pairs each tag_name with the very
  # next created_at (the release's own, not its assets'), then we sort.
  local body tag
  body="$(http_get_stdout "https://api.github.com/repos/$REPO/releases")"
  tag="$(printf '%s' "$body" | awk '
    /"tag_name":[[:space:]]*"/ {
      match($0, /"tag_name":[[:space:]]*"[^"]+"/)
      s = substr($0, RSTART, RLENGTH)
      sub(/"tag_name":[[:space:]]*"/, "", s)
      sub(/"$/, "", s)
      tag = s
      expecting = 1
      next
    }
    /"created_at":[[:space:]]*"/ {
      if (expecting) {
        match($0, /"created_at":[[:space:]]*"[^"]+"/)
        s = substr($0, RSTART, RLENGTH)
        sub(/"created_at":[[:space:]]*"/, "", s)
        sub(/"$/, "", s)
        printf "%s\t%s\n", s, tag
        expecting = 0
      }
    }
  ' | sort -r | head -1 | cut -f2-)"
  [[ -n "$tag" ]] || die "could not resolve latest version (is the repo public and has releases?)"
  echo "$tag"
}

pick_install_dir() {
  if [[ -n "$INSTALL_DIR" ]]; then
    mkdir -p "$INSTALL_DIR"
    echo "$INSTALL_DIR"
    return
  fi
  if [[ -w /usr/local/bin ]]; then
    echo /usr/local/bin
    return
  fi
  local d="$HOME/.local/bin"
  mkdir -p "$d"
  echo "$d"
}

verify_sha256() {
  # verify_sha256 FILE EXPECTED_HEX
  local file="$1" expected="$2" got
  if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "$file" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    got="$(shasum -a 256 "$file" | awk '{print $1}')"
  else
    die "need sha256sum or shasum to verify download"
  fi
  if [[ "$got" != "$expected" ]]; then
    die "checksum mismatch for $(basename "$file"): got $got, expected $expected"
  fi
}

main() {
  need uname
  need tar
  need awk

  local os arch version asset url base tmp
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(resolve_version "$VERSION")"
  # GoReleaser strips the leading 'v' from {{ .Version }} in archive names.
  local stripped="${version#v}"
  asset="${BIN}_${stripped}_${os}_${arch}.tar.gz"
  base="https://github.com/$REPO/releases/download/$version"

  log "installing $BIN $version for $os/$arch"

  tmp="$(mktemp -d)"
  # Use double quotes so $tmp expands now, when the trap is set — not when it
  # fires (by which point `tmp` is out of scope and would trip `set -u`).
  trap "rm -rf '$tmp'" EXIT

  log "downloading $asset"
  http_get "$base/$asset" "$tmp/$asset"

  log "downloading checksums.txt"
  http_get "$base/checksums.txt" "$tmp/checksums.txt"

  local expected
  expected="$(awk -v f="$asset" '$2==f {print $1}' "$tmp/checksums.txt")"
  [[ -n "$expected" ]] || die "no checksum entry for $asset"
  verify_sha256 "$tmp/$asset" "$expected"
  log "checksum OK"

  tar -xzf "$tmp/$asset" -C "$tmp"
  [[ -f "$tmp/$BIN" ]] || die "archive did not contain expected binary '$BIN'"

  local dest
  dest="$(pick_install_dir)"
  install -m 0755 "$tmp/$BIN" "$dest/$BIN"
  log "installed → $dest/$BIN"

  case ":$PATH:" in
    *":$dest:"*) ;;
    *) warn "$dest is not on your PATH — add it, e.g.: echo 'export PATH=\"$dest:\$PATH\"' >> ~/.bashrc" ;;
  esac

  log "$("$dest/$BIN" --version 2>/dev/null || echo "$BIN $version")"
}

main "$@"
