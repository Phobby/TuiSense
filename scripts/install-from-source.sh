#!/usr/bin/env bash
set -euo pipefail

APP_NAME="tuisense"
PREFIX="${PREFIX:-$HOME/.local}"
INSTALL_DIR="${BINDIR:-$PREFIX/bin}"
TARGET_BIN="$INSTALL_DIR/$APP_NAME"

XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_BIN="$PROJECT_ROOT/bin/$APP_NAME"

ACTION="install"
PURGE=0

for arg in "$@"; do
    case "$arg" in
        --uninstall|uninstall)
            ACTION="uninstall"
            ;;
        --purge)
            PURGE=1
            ;;
        -h|--help)
            cat <<EOF
TuiSense Installer for Linux

Usage (developers; release installs use scripts/install.sh):
  ./scripts/install-from-source.sh               Build and install tuisense to ~/.local/bin
  ./scripts/install-from-source.sh --uninstall   Remove ~/.local/bin/tuisense
  ./scripts/install-from-source.sh --uninstall --purge  Also remove ~/.config/tuisense & cache
EOF
            exit 0
            ;;
    esac
done

ensure_path_in_shell_rc() {
    local target_dir="$1"
    local user_shell
    user_shell="$(basename "${SHELL:-sh}")"

    # Snippet that safely prepends target_dir only if not already present in PATH
    local posix_snippet
    posix_snippet=$(cat <<'EOF'
# >>> tuisense path >>>
case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) export PATH="$HOME/.local/bin:$PATH" ;;
esac
# <<< tuisense path <<<
EOF
)

    local rc_file=""
    case "$user_shell" in
        bash)
            rc_file="$HOME/.bashrc"
            ;;
        zsh)
            rc_file="$HOME/.zshrc"
            ;;
        fish)
            rc_file="$HOME/.config/fish/config.fish"
            ;;
        *)
            rc_file="$HOME/.profile"
            ;;
    esac

    if [ "$user_shell" = "fish" ]; then
        mkdir -p "$(dirname "$rc_file")"
        touch "$rc_file"
        if ! grep -Fq "tuisense path" "$rc_file" && ! grep -Fq ".local/bin" "$rc_file"; then
            cat >> "$rc_file" <<'EOF'

# >>> tuisense path >>>
if not contains "$HOME/.local/bin" $PATH
    fish_add_path -g "$HOME/.local/bin"
end
# <<< tuisense path <<<
EOF
            echo "[tuisense] Added $target_dir to $rc_file (fish)"
        fi
        return
    fi

    if [ -n "$rc_file" ]; then
        touch "$rc_file"
        if ! grep -Fq "tuisense path" "$rc_file" && ! grep -Fq ".local/bin" "$rc_file"; then
            printf "\n%s\n" "$posix_snippet" >> "$rc_file"
            echo "[tuisense] Added $target_dir to $rc_file ($user_shell)"
        fi
    fi
}

do_install() {
    echo "[tuisense] Building standalone executable..."
    mkdir -p "$PROJECT_ROOT/bin"
    (
        cd "$PROJECT_ROOT"
        go build -trimpath -ldflags "-s -w -X github.com/Phobby/TuiSense/internal/version.Version=$(cat VERSION)" -o "$BUILD_BIN" ./cmd/tuisense
    )

    mkdir -p "$INSTALL_DIR"
    mkdir -p "$XDG_CONFIG_HOME/tuisense/profiles"
    mkdir -p "$XDG_DATA_HOME/tuisense"
    mkdir -p "$XDG_CACHE_HOME/tuisense"

    # Atomic installation so running instances or existing binaries are cleanly replaced
    local tmp_bin="$INSTALL_DIR/.${APP_NAME}.tmp.$$"
    install -m 0755 "$BUILD_BIN" "$tmp_bin"
    mv -f "$tmp_bin" "$TARGET_BIN"

    ensure_path_in_shell_rc "$INSTALL_DIR"

    echo "[tuisense] Installed binary to: $TARGET_BIN"
    echo "[tuisense] Config directory:    $XDG_CONFIG_HOME/tuisense/profiles"

    case ":$PATH:" in
        *":$INSTALL_DIR:"*)
            echo "[tuisense] '$INSTALL_DIR' is active in your PATH."
            ;;
        *)
            echo "[tuisense] Note: Run 'export PATH=\"$INSTALL_DIR:\$PATH\"' or open a new terminal session."
            ;;
    esac
}

do_uninstall() {
    if [ -f "$TARGET_BIN" ]; then
        rm -f "$TARGET_BIN"
        echo "[tuisense] Removed $TARGET_BIN"
    else
        echo "[tuisense] Binary not found at $TARGET_BIN (already uninstalled)"
    fi

    if [ "$PURGE" -eq 1 ]; then
        rm -rf "$XDG_CONFIG_HOME/tuisense" "$XDG_DATA_HOME/tuisense" "$XDG_CACHE_HOME/tuisense"
        echo "[tuisense] Purged configuration and cache directories."
    else
        echo "[tuisense] Preserved user profiles in $XDG_CONFIG_HOME/tuisense/profiles"
    fi
}

if [ "$ACTION" = "uninstall" ]; then
    do_uninstall
else
    do_install
fi
