#!/bin/sh
# Verification-first install and update script for git-governance.
#
# Install and update are one idempotent mechanism: re-running this script
# installs the pinned version, or the latest release when no pin is given.
# The update is the install re-run through the same channel; there is no
# separate update path and no automatic self-update.
#
# The script implements the verification-first standard of
# docs/operations/installation-and-release.md (section 9.1) and
# docs/architecture/ADR-0009-CHANNEL-OWNED-DELIVERY-AND-UPDATE-MODEL.md:
#
#   1. Resolve the version from an explicit pin or a validated latest
#      resolution; an unvalidated version is never installed.
#   2. Download the release artifact, its SHA-256 checksum manifest, and its
#      signature bundle, and verify fail-closed before any mutation. The
#      release checksum manifest is the single verification truth; this
#      script introduces no second checksum scheme.
#   3. The signature bundle is verified with cosign when cosign is installed.
#      When cosign is not installed the script reports this explicitly instead
#      of assuming an additional consumer dependency; the checksum
#      verification stays mandatory in every case.
#   4. Write the binary to a temporary path, run an executable smoke test,
#      replace the target, and preserve the previous version as
#      git-governance.previous for a controlled rollback.
#   5. Report whether the current terminal already knows the install
#      directory and recommend a new terminal when needed. Shell profiles are
#      never edited by this script.
#
# Usage:
#
#   curl -fsSL https://raw.githubusercontent.com/t33n-software/git-governance/main/install.sh | sh
#
#   or from a checkout:
#
#   sh install.sh
#
# Environment overrides:
#
#   GIT_GOVERNANCE_VERSION      pin (e.g. 1.2.3 or v1.2.3); latest when unset
#   GIT_GOVERNANCE_INSTALL_DIR  target directory (default ~/.local/bin)
#
# Manual zero-dependency path: download the artifact, the checksum manifest,
# and the signature bundle from
# https://github.com/t33n-software/git-governance/releases and follow section
# 5.2 of docs/operations/installation-and-release.md.
#
# Portability note: this script is written for POSIX sh on purpose so it runs
# under bash, dash, zsh, and busybox ash alike; it uses no bashisms.

set -eu

REPO_OWNER="t33n-software"
REPO_NAME="git-governance"
REPO_SLUG="${REPO_OWNER}/${REPO_NAME}"
RELEASES_URL="https://github.com/${REPO_SLUG}/releases"
DOWNLOAD_URL="${RELEASES_URL}/download"
API_URL="https://api.github.com/repos/${REPO_SLUG}/releases/latest"
BINARY_NAME="git-governance"

log() {
    printf '%s\n' "$*"
}

warn() {
    printf 'warning: %s\n' "$*" >&2
}

fail() {
    printf 'error: %s\n' "$*" >&2
    exit 1
}

need_cmd() {
    command -v "$1" >/dev/null 2>&1
}

# --- temporary workspace -----------------------------------------------------
need_cmd mktemp || fail "mktemp is required but was not found; use the manual download path: ${RELEASES_URL}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/git-governance-install.XXXXXX")"
cleanup() {
    rm -rf "$WORK_DIR"
}
trap cleanup EXIT
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM

# --- fetcher detection (detect, never assume) --------------------------------
if need_cmd curl; then
    FETCHER="curl"
elif need_cmd wget; then
    FETCHER="wget"
else
    fail "neither curl nor wget is available; install curl with your package manager (for example 'apt install curl' or 'brew install curl') or use the manual download path: ${RELEASES_URL}"
fi

fetch() {
    # fetch <url> <destination-file>
    case "${FETCHER}" in
        curl)
            if ! curl -fSL --retry 3 --proto '=https' -o "$2" "$1"; then
                fail "download failed: $1"
            fi
            ;;
        wget)
            if ! wget -q --tries=3 -O "$2" "$1"; then
                fail "download failed: $1"
            fi
            ;;
    esac
}

# --- platform detection ------------------------------------------------------
case "$(uname -s)" in
    Linux)
        OS="linux"
        ;;
    Darwin)
        OS="darwin"
        ;;
    *)
        fail "unsupported operating system '$(uname -s)'; use the manual download path: ${RELEASES_URL}"
        ;;
esac

case "$(uname -m)" in
    x86_64 | amd64)
        ARCH="amd64"
        ;;
    arm64 | aarch64)
        ARCH="arm64"
        ;;
    *)
        fail "unsupported architecture '$(uname -m)'; use the manual download path: ${RELEASES_URL}"
        ;;
esac

# --- version resolution ------------------------------------------------------
is_semver() {
    printf '%s\n' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'
}

strip_v() {
    v="$1"
    case "$v" in
        v*) v="${v#v}" ;;
    esac
    printf '%s' "$v"
}

PIN="${GIT_GOVERNANCE_VERSION:-}"
if [ -n "$PIN" ]; then
    VERSION="$(strip_v "$PIN")"
    is_semver "$VERSION" || fail "invalid pinned version '${PIN}'; expected SemVer such as 1.2.3"
    log "Installing pinned release: v${VERSION}"
else
    log "Resolving the latest release ..."
    fetch "$API_URL" "${WORK_DIR}/release.json"
    TAG="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\(v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)".*/\1/p' "${WORK_DIR}/release.json" | head -n 1)"
    [ -n "$TAG" ] || fail "could not resolve the latest release tag; pin a version with GIT_GOVERNANCE_VERSION or use the manual path: ${RELEASES_URL}"
    VERSION="$(strip_v "$TAG")"
    is_semver "$VERSION" || fail "resolved tag '${TAG}' is not a valid SemVer release; refusing to continue"
    log "Resolved latest release: ${TAG}"
fi

# --- artifacts (single verification truth: the release checksum manifest) ----
ARCHIVE="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
CHECKSUMS="${BINARY_NAME}_${VERSION}_checksums.txt"
SIGNATURE="${CHECKSUMS}.sigstore.json"

log "Downloading ${ARCHIVE}, ${CHECKSUMS}, and the signature bundle ..."
fetch "${DOWNLOAD_URL}/v${VERSION}/${ARCHIVE}" "${WORK_DIR}/${ARCHIVE}"
fetch "${DOWNLOAD_URL}/v${VERSION}/${CHECKSUMS}" "${WORK_DIR}/${CHECKSUMS}"
fetch "${DOWNLOAD_URL}/v${VERSION}/${SIGNATURE}" "${WORK_DIR}/${SIGNATURE}"

# --- checksum verification (mandatory, native, fail-closed) ------------------
if need_cmd sha256sum; then
    compute_hash() {
        sha256sum "$1"
    }
elif need_cmd shasum; then
    compute_hash() {
        shasum -a 256 "$1"
    }
else
    fail "neither sha256sum nor shasum is available; verify the SHA-256 checksum manually and use the manual download path: ${RELEASES_URL}"
fi

EXPECTED="$(awk -v name="${ARCHIVE}" '$2 == name { print $1 }' "${WORK_DIR}/${CHECKSUMS}")"
[ -n "$EXPECTED" ] || fail "checksum manifest does not contain an entry for ${ARCHIVE}; refusing to continue"
printf '%s\n' "$EXPECTED" | grep -Eq '^[0-9a-fA-F]{64}$' || fail "checksum manifest entry for ${ARCHIVE} is malformed; refusing to continue"
ACTUAL="$(compute_hash "${WORK_DIR}/${ARCHIVE}" | awk '{print $1}')"
[ "$ACTUAL" = "$EXPECTED" ] || fail "checksum mismatch for ${ARCHIVE}: expected ${EXPECTED}, got ${ACTUAL}; refusing to install"
log "Checksum verified: ${ARCHIVE}"

# --- signature verification (cosign when present, explicit when absent) ------
if need_cmd cosign; then
    log "Verifying the signature bundle with cosign ..."
    if ! cosign verify-blob \
        --bundle "${WORK_DIR}/${SIGNATURE}" \
        --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
        --certificate-identity-regexp "^https://github.com/${REPO_SLUG}/" \
        "${WORK_DIR}/${CHECKSUMS}"; then
        fail "signature verification failed for ${CHECKSUMS}; refusing to install"
    fi
    log "Signature verified: ${CHECKSUMS}"
else
    warn "cosign is not installed; the signature bundle was downloaded but not verified locally. The SHA-256 checksum manifest (the release verification truth) was verified natively. To verify the signature, install cosign and run:"
    warn "cosign verify-blob --bundle ${SIGNATURE} --certificate-oidc-issuer https://token.actions.githubusercontent.com --certificate-identity-regexp '^https://github.com/${REPO_SLUG}/' ${CHECKSUMS}"
fi

# --- extraction and smoke test ------------------------------------------------
log "Extracting ${ARCHIVE} ..."
tar -xzf "${WORK_DIR}/${ARCHIVE}" -C "${WORK_DIR}"
[ -f "${WORK_DIR}/${BINARY_NAME}" ] || fail "the archive does not contain ${BINARY_NAME}; refusing to install"
chmod +x "${WORK_DIR}/${BINARY_NAME}"

log "Running the executable smoke test ..."
SMOKE="$("${WORK_DIR}/${BINARY_NAME}" --version)" || fail "the downloaded binary failed the --version smoke test; refusing to install"
[ -n "$SMOKE" ] || fail "the downloaded binary returned an empty --version; refusing to install"
log "Smoke test passed: ${SMOKE}"

# --- atomic installation with rollback copy -----------------------------------
INSTALL_DIR="${GIT_GOVERNANCE_INSTALL_DIR:-${HOME}/.local/bin}"
mkdir -p "$INSTALL_DIR"
TARGET="${INSTALL_DIR}/${BINARY_NAME}"

if [ -e "$TARGET" ]; then
    cp "$TARGET" "${TARGET}.previous"
    log "Previous version preserved: ${TARGET}.previous"
fi

mv -f "${WORK_DIR}/${BINARY_NAME}" "$TARGET"
log "Installed: ${TARGET}"

INSTALLED="$("$TARGET" --version)" || fail "the installed binary failed --version verification; restore the previous version from ${TARGET}.previous if present"
[ "$INSTALLED" = "$SMOKE" ] || warn "the installed --version output differs from the smoke test output; inspect with ${TARGET} --version"
log "Verification: ${INSTALLED}"

if "$TARGET" doctor >/dev/null 2>&1; then
    log "Diagnostics: doctor completed successfully"
else
    warn "doctor reported issues; its diagnostics include repository checks that depend on the current working directory. Run '${BINARY_NAME} doctor' inside your project to review them."
fi

# --- PATH and session reporting (no profile edits, ever) ----------------------
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*)
        log "PATH: ${INSTALL_DIR} is already known to this terminal"
        ;;
    *)
        log "PATH: ${INSTALL_DIR} is NOT on the PATH of the current terminal."
        log "  For this session:  export PATH=\"${INSTALL_DIR}:\$PATH\""
        log "  Permanently:       add the line above to your shell profile yourself"
        log "  Then open a new terminal so every new process finds ${BINARY_NAME}."
        log "  This script never edits shell profiles."
        ;;
esac

log ""
log "Installation complete: ${BINARY_NAME} ${VERSION} (${OS}/${ARCH})"
log "  Binary:   ${TARGET}"
log "  Update:   re-run this script (the update is the install re-run)"
log "  Manual:   ${RELEASES_URL}"