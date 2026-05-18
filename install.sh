#!/bin/sh
#
# install.sh — one-line installer for the AraraHQ CLI.
#
# Usage (default: latest stable, /usr/local/bin):
#     curl -fsSL https://raw.githubusercontent.com/ararahq/cli/main/install.sh | sh
#
# Customize via env vars:
#     ARARA_VERSION       pin a specific tag (e.g. v0.2.0). Default: latest release.
#     ARARA_INSTALL_DIR   where the binary lands. Default: /usr/local/bin (or ~/.local/bin if
#                         /usr/local/bin is not writable without sudo).
#     ARARA_NO_VERIFY     set to 1 to skip the SHA256 verification step (NOT recommended).
#
# Exit codes:
#     0   installed successfully
#     1   unsupported OS / architecture
#     2   download or checksum failure
#     3   install directory not writable and no fallback available
#
# Modeled after the install scripts of Stripe CLI, Bun, and Deno — POSIX sh,
# no bash-isms, no jq, no Python. Pure curl + tar + shasum.

set -eu

REPO="ararahq/cli"
BINARY="arara"

# ── pretty output ────────────────────────────────────────────────────────
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_RESET="$(printf '\033[0m')"
    C_BRAND="$(printf '\033[38;2;28;153;167m')"   # .pen brand-500
    C_DIM="$(printf '\033[38;2;118;160;166m')"    # .pen text-secondary
    C_OK="$(printf '\033[38;2;16;185;129m')"      # .pen success
    C_WARN="$(printf '\033[38;2;245;158;11m')"    # .pen warning
    C_ERR="$(printf '\033[38;2;239;68;68m')"      # .pen danger
else
    C_RESET="" C_BRAND="" C_DIM="" C_OK="" C_WARN="" C_ERR=""
fi

say()   { printf '%s▸%s %s\n'  "$C_BRAND" "$C_RESET" "$*"; }
ok()    { printf '%s✓%s %s\n'  "$C_OK"    "$C_RESET" "$*"; }
warn()  { printf '%s!%s %s\n'  "$C_WARN"  "$C_RESET" "$*"; }
die()   { printf '%s✘%s %s\n'  "$C_ERR"   "$C_RESET" "$*" 1>&2; exit "${2:-1}"; }

# ── detect platform ──────────────────────────────────────────────────────
detect_os() {
    case "$(uname -s)" in
        Darwin)  echo "darwin"  ;;
        Linux)   echo "linux"   ;;
        *)       die "Unsupported OS: $(uname -s). Use the Windows installer at https://github.com/${REPO}#install" 1 ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *)              die "Unsupported architecture: $(uname -m)" 1 ;;
    esac
}

# ── network helpers ──────────────────────────────────────────────────────
have() { command -v "$1" >/dev/null 2>&1; }

# Resolve the latest release tag via the GitHub API. The script must not
# depend on `jq` (won't be installed on a fresh box), so we grep the field
# out of the JSON — the field is single-line and never escaped.
latest_tag() {
    api_url="https://api.github.com/repos/${REPO}/releases/latest"
    if have curl; then
        curl -fsSL "$api_url"
    elif have wget; then
        wget -qO- "$api_url"
    else
        die "Neither curl nor wget is available — install one and re-run." 2
    fi | grep -m1 '"tag_name":' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
}

download() {
    url="$1"; out="$2"
    if have curl; then
        curl -fsSL --retry 3 --retry-delay 1 -o "$out" "$url" \
            || die "Failed to download ${url}" 2
    else
        wget -q --tries=3 -O "$out" "$url" \
            || die "Failed to download ${url}" 2
    fi
}

# ── checksum verification ────────────────────────────────────────────────
verify_checksum() {
    archive_path="$1"; checksums_path="$2"; archive_name="$3"
    if [ -n "${ARARA_NO_VERIFY:-}" ]; then
        warn "ARARA_NO_VERIFY set — skipping SHA256 verification."
        return 0
    fi

    expected="$(grep " ${archive_name}\$" "$checksums_path" | awk '{print $1}')"
    if [ -z "$expected" ]; then
        die "Checksum for ${archive_name} not found in checksums.txt" 2
    fi

    if have sha256sum; then
        actual="$(sha256sum "$archive_path" | awk '{print $1}')"
    elif have shasum; then
        actual="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
    else
        warn "No sha256sum/shasum found — skipping verification."
        return 0
    fi

    if [ "$expected" != "$actual" ]; then
        die "Checksum mismatch — expected ${expected}, got ${actual}. Aborting." 2
    fi
}

# ── install destination ──────────────────────────────────────────────────
# Prefer the user-supplied dir; otherwise pick the first writable location
# in /usr/local/bin → ~/.local/bin → ~/bin. We never invoke sudo: if the
# user wants global install they can re-run with `sudo sh -c "curl … | sh"`.
resolve_install_dir() {
    if [ -n "${ARARA_INSTALL_DIR:-}" ]; then
        echo "$ARARA_INSTALL_DIR"
        return
    fi
    for candidate in /usr/local/bin "$HOME/.local/bin" "$HOME/bin"; do
        if [ -d "$candidate" ] && [ -w "$candidate" ]; then
            echo "$candidate"
            return
        fi
        # Allow creating ~/.local/bin or ~/bin on first install — common on
        # fresh user homes where neither exists yet.
        if [ "$candidate" != "/usr/local/bin" ] && ! [ -e "$candidate" ]; then
            mkdir -p "$candidate" 2>/dev/null && echo "$candidate" && return
        fi
    done
    die "No writable install dir found. Re-run with ARARA_INSTALL_DIR=... or use sudo." 3
}

# ── PATH hint ────────────────────────────────────────────────────────────
# After installing to ~/.local/bin or ~/bin we tell the user to add the
# dir to PATH if it isn't already — silently breaking their first `arara`
# command would be a terrible first impression.
hint_path() {
    install_dir="$1"
    case ":$PATH:" in
        *":$install_dir:"*) return ;;
    esac
    warn "${install_dir} is not on your PATH."
    printf '  Add this line to your shell profile (~/.zshrc / ~/.bashrc):\n'
    printf '    %sexport PATH="%s:$PATH"%s\n' "$C_BRAND" "$install_dir" "$C_RESET"
    printf '  Then reopen your terminal or run: %ssource ~/.zshrc%s\n' "$C_BRAND" "$C_RESET"
}

# ── main ─────────────────────────────────────────────────────────────────
main() {
    say "Installing AraraHQ CLI"

    os="$(detect_os)"
    arch="$(detect_arch)"
    tag="${ARARA_VERSION:-$(latest_tag)}"
    [ -z "$tag" ] && die "Could not determine latest release tag." 2

    # Strip leading 'v' so the filename interpolation matches goreleaser's
    # `{{ .Version }}` (which is the tag without the 'v').
    version="${tag#v}"
    archive_name="${BINARY}_${version}_${os}_${arch}.tar.gz"
    archive_url="https://github.com/${REPO}/releases/download/${tag}/${archive_name}"
    checksums_url="https://github.com/${REPO}/releases/download/${tag}/checksums.txt"

    say "Platform: ${os}/${arch}    Version: ${tag}"

    tmp="$(mktemp -d 2>/dev/null || mktemp -d -t arara-install)"
    # shellcheck disable=SC2064 # we want the cleanup path captured now
    trap "rm -rf '$tmp'" EXIT INT TERM

    download "$archive_url"    "$tmp/$archive_name"
    download "$checksums_url"  "$tmp/checksums.txt"
    verify_checksum "$tmp/$archive_name" "$tmp/checksums.txt" "$archive_name"
    ok "Downloaded and verified ${archive_name}"

    ( cd "$tmp" && tar -xzf "$archive_name" ) \
        || die "Failed to extract archive." 2
    [ -f "$tmp/${BINARY}" ] || die "Archive did not contain ${BINARY} binary." 2
    chmod +x "$tmp/${BINARY}"

    install_dir="$(resolve_install_dir)"
    mv "$tmp/${BINARY}" "$install_dir/${BINARY}" \
        || die "Failed to move binary to ${install_dir}." 3
    ok "Installed to ${install_dir}/${BINARY}"

    hint_path "$install_dir"

    # Print version to confirm the install worked end-to-end. If the
    # binary refuses to run (libc mismatch on exotic distros) the user
    # sees the error here instead of much later.
    if "$install_dir/${BINARY}" --version >/dev/null 2>&1; then
        installed_version="$("$install_dir/${BINARY}" --version)"
        ok "${installed_version}"
        printf '\n%sNext:%s run %sarara login%s to authenticate.\n' \
            "$C_BRAND" "$C_RESET" "$C_BRAND" "$C_RESET"
    else
        warn "Installed binary failed to run. Open an issue at https://github.com/${REPO}/issues"
        exit 2
    fi
}

main "$@"
