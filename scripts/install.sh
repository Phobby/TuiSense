#!/bin/sh
# TuiSense installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/Phobby/TuiSense/main/scripts/install.sh | sh
#
# Safer: download, read, then run it:
#
#   curl -fsSLo install-tuisense.sh https://raw.githubusercontent.com/Phobby/TuiSense/main/scripts/install.sh
#   less install-tuisense.sh && sh install-tuisense.sh
#
# What it does: detects your OS and CPU, downloads the official release
# archive from GitHub, verifies the signed checksum list (OpenSSH signature
# against the key pinned below) and the archive's SHA-256, then installs the
# `tuisense` binary into ~/.local/bin without root. Nothing is run before it is
# verified. Settings, profiles and reports are never touched.
#
# Options:
#   --version X.Y.Z   install a specific release (default: latest stable)
#   --dir PATH        install directory (default: ~/.local/bin)
#   --no-modify-path  do not add the directory to your shell profile
#   --uninstall       remove the binary (keeps settings and profiles)
#   --purge           with --uninstall, also remove settings, profiles and data
set -eu

REPO="Phobby/TuiSense"
NAMESPACE="tuisense-release"
# Pinned release signing keys (ssh-ed25519). Keep in sync with
# internal/update/trusted_keys.txt and scripts/install.ps1.
TRUSTED_KEYS="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILqV1bC0kFEN1Vxj3JnEp5ZieGsstZf0Atekky5WXFyv tuisense-release"

API_URL="${TUISENSE_API_URL:-https://api.github.com}"
DOWNLOAD_URL="${TUISENSE_DOWNLOAD_URL:-https://github.com/$REPO/releases/download}"
INSTALL_DIR="${TUISENSE_INSTALL_DIR:-$HOME/.local/bin}"
VERSION=""
ACTION=install
PURGE=0
MODIFY_PATH=1

say() { printf '[tuisense] %s\n' "$*"; }
die() { printf '[tuisense] error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        --version) [ $# -ge 2 ] || die "--version needs a value"; VERSION="${2#v}"; shift ;;
        --dir) [ $# -ge 2 ] || die "--dir needs a value"; INSTALL_DIR="$2"; shift ;;
        --no-modify-path) MODIFY_PATH=0 ;;
        --uninstall) ACTION=uninstall ;;
        --purge) PURGE=1 ;;
        -h|--help) sed -n '2,24p' "$0" 2>/dev/null || true; exit 0 ;;
        *) die "unknown option: $1" ;;
    esac
    shift
done

TARGET="$INSTALL_DIR/tuisense"

if [ "$ACTION" = uninstall ]; then
    rm -f "$TARGET" "$TARGET.old" "$TARGET.new"
    say "Removed $TARGET"
    config="${XDG_CONFIG_HOME:-$HOME/.config}/tuisense"
    data="${XDG_DATA_HOME:-$HOME/.local/share}/tuisense"
    if [ "$(uname -s)" = Darwin ] && [ -z "${XDG_CONFIG_HOME:-}" ]; then
        config="$HOME/Library/Application Support/tuisense"
    fi
    if [ "$PURGE" -eq 1 ]; then
        rm -rf "$config" "$data" "${XDG_CACHE_HOME:-$HOME/.cache}/tuisense"
        say "Removed settings, profiles and data."
    else
        say "Kept settings and profiles in $config"
    fi
    exit 0
fi

# --- platform ---------------------------------------------------------------
os="${TUISENSE_OS:-$(uname -s)}"
arch="${TUISENSE_ARCH:-$(uname -m)}"
case "$os" in
    Linux|linux) os=linux ;;
    Darwin|darwin) os=darwin ;;
    *) die "unsupported OS '$os' (on Windows use scripts/install.ps1 in PowerShell)" ;;
esac
case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "unsupported CPU architecture '$arch'" ;;
esac

# --- tools ------------------------------------------------------------------
PROTO='=https'
case "$API_URL$DOWNLOAD_URL" in *http://*) PROTO='=https,http' ;; esac  # local test servers only
if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL --proto "$PROTO" --proto-redir "$PROTO" --retry 2 -o "$2" "$1"; }
    fetch_stdout() { curl -fsSL --proto "$PROTO" --proto-redir "$PROTO" --retry 2 "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -q -O "$2" "$1"; }
    fetch_stdout() { wget -q -O - "$1"; }
else
    die "curl or wget is required"
fi
if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
    die "sha256sum or shasum is required"
fi
command -v tar >/dev/null 2>&1 || die "tar is required"
command -v ssh-keygen >/dev/null 2>&1 || die "ssh-keygen (OpenSSH) is required to verify the release signature"

# --- trust anchor -----------------------------------------------------------
keys="$TRUSTED_KEYS"
if [ -n "${TUISENSE_TRUSTED_KEYS_FILE:-}" ]; then
    # Testing hook: extra keys (the environment is already trusted to run this script).
    keys="$keys
$(cat "$TUISENSE_TRUSTED_KEYS_FILE")"
fi
if ! printf '%s\n' "$keys" | grep -q 'ssh-ed25519'; then
    die "this installer has no pinned release signing key yet, so it cannot verify a download. Build from source instead (see README)."
fi

tmp="$(mktemp -d 2>/dev/null || mktemp -d -t tuisense)"
trap 'rm -rf "$tmp"' EXIT INT TERM

# --- release ----------------------------------------------------------------
if [ -z "$VERSION" ]; then
    meta="$(fetch_stdout "$API_URL/repos/$REPO/releases/latest" 2>/dev/null)" \
        || die "no published TuiSense release was found (or GitHub could not be reached). See https://github.com/$REPO/releases"
    VERSION="$(printf '%s' "$meta" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' | head -n 1)"
    [ -n "$VERSION" ] || die "could not determine the latest release"
fi
case "$VERSION" in
    *[!0-9A-Za-z.-]*|'') die "invalid version '$VERSION'" ;;
esac
archive="tuisense_${VERSION}_${os}_${arch}.tar.gz"
base="$DOWNLOAD_URL/v$VERSION"
say "Installing TuiSense v$VERSION for $os/$arch"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || die "download failed: SHA256SUMS"
fetch "$base/SHA256SUMS.sig" "$tmp/SHA256SUMS.sig" || die "download failed: SHA256SUMS.sig"
fetch "$base/$archive" "$tmp/$archive" || die "no release package for $os/$arch ($archive)"

# --- verification -----------------------------------------------------------
printf '%s\n' "$keys" | grep 'ssh-ed25519' | while IFS= read -r line; do
    key="$(printf '%s\n' "$line" | sed -n 's/.*\(ssh-ed25519 [A-Za-z0-9+/=]*\).*/\1/p')"
    printf '%s %s\n' "$NAMESPACE" "$key"
done > "$tmp/allowed_signers"
ssh-keygen -Y verify -f "$tmp/allowed_signers" -I "$NAMESPACE" -n "$NAMESPACE" \
    -s "$tmp/SHA256SUMS.sig" < "$tmp/SHA256SUMS" >/dev/null 2>&1 \
    || die "the checksum list is not signed by a trusted TuiSense key; nothing was installed"
expected="$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")"
[ -n "$expected" ] || die "$archive is not listed in the signed checksums"
actual="$(sha256 "$tmp/$archive")"
[ "$expected" = "$actual" ] || die "checksum mismatch for $archive; nothing was installed"
say "Signature and checksum verified."

mkdir "$tmp/x"
tar -xzf "$tmp/$archive" -C "$tmp/x"
bin="$tmp/x/tuisense_${VERSION}_${os}_${arch}/tuisense"
[ -f "$bin" ] || die "the package does not contain the tuisense executable"
chmod 0755 "$bin"
"$bin" --version 2>/dev/null | grep -q "v$VERSION" || die "the downloaded executable did not start correctly; nothing was installed"

# --- install (atomic; the previous binary is kept as .old) -------------------
mkdir -p "$INSTALL_DIR"
cp "$bin" "$TARGET.new"
chmod 0755 "$TARGET.new"
if [ -f "$TARGET" ]; then
    mv -f "$TARGET" "$TARGET.old"
fi
mv -f "$TARGET.new" "$TARGET"
say "Installed $TARGET"

# --- PATH -------------------------------------------------------------------
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        if [ "$MODIFY_PATH" -eq 1 ]; then
            case "$(basename "${SHELL:-sh}")" in
                zsh) rc="$HOME/.zshrc" ;;
                bash) rc="$HOME/.bashrc"; [ "$os" = darwin ] && rc="$HOME/.bash_profile" ;;
                fish) rc="" ;;
                *) rc="$HOME/.profile" ;;
            esac
            if [ -z "$rc" ]; then
                say "Add $INSTALL_DIR to your PATH: fish_add_path $INSTALL_DIR"
            elif ! grep -Fq "# tuisense path" "$rc" 2>/dev/null; then
                printf '\nexport PATH="%s:$PATH" # tuisense path\n' "$INSTALL_DIR" >> "$rc"
                say "Added $INSTALL_DIR to PATH in $rc (open a new terminal)"
            fi
        else
            say "Add $INSTALL_DIR to your PATH to run tuisense from anywhere."
        fi
        ;;
esac

"$TARGET" --version
say "Run: tuisense   (help: tuisense --help, checks: tuisense doctor)"
