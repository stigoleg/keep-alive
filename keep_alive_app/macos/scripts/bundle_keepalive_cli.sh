#!/bin/sh
# Builds the keepalive Go CLI and embeds it into the macOS app bundle's
# Resources directory so the app can launch it from a known, signed path
# instead of relying on PATH/Homebrew. Falls back gracefully when go is
# not available so non-developer builds still link.

set -e

REPO_ROOT="${SRCROOT}/.."
if [ ! -f "${REPO_ROOT}/go.mod" ]; then
    REPO_ROOT="${SRCROOT}/../.."
fi

CMD_DIR="${REPO_ROOT}/cmd/keepalive"
if [ ! -d "${CMD_DIR}" ]; then
    echo "warning: cmd/keepalive not found at ${CMD_DIR}; skipping bundled CLI build"
    exit 0
fi

DEST_DIR="${BUILT_PRODUCTS_DIR}/${CONTENTS_FOLDER_PATH}/Resources"
DEST="${DEST_DIR}/keepalive"
mkdir -p "${DEST_DIR}"

GO_BIN=""
for candidate in \
    "${HOME}/go/bin/go" \
    "/opt/homebrew/bin/go" \
    "/usr/local/bin/go" \
    "/usr/local/go/bin/go" \
    "$(command -v go 2>/dev/null || true)"
do
    if [ -n "${candidate}" ] && [ -x "${candidate}" ]; then
        GO_BIN="${candidate}"
        break
    fi
done

if [ -z "${GO_BIN}" ]; then
    echo "warning: go toolchain not found; bundled CLI will be omitted"
    exit 0
fi

case "${ARCHS}" in
    *arm64*) GOARCH="arm64" ;;
    *x86_64*) GOARCH="amd64" ;;
    *) GOARCH="$(uname -m | sed 's/aarch64/arm64/;s/x86_64/amd64/')" ;;
esac

echo "Building keepalive CLI (${GOARCH}) into ${DEST}"
(
    cd "${REPO_ROOT}"
    CGO_ENABLED=1 GOOS=darwin GOARCH="${GOARCH}" \
        "${GO_BIN}" build -trimpath -o "${DEST}" ./cmd/keepalive
)
chmod +x "${DEST}"

SIGN_IDENTITY="${EXPANDED_CODE_SIGN_IDENTITY:-${CODE_SIGN_IDENTITY:-}}"
if [ -z "${SIGN_IDENTITY}" ] || [ "${SIGN_IDENTITY}" = "-" ]; then
    /usr/bin/codesign --force --sign - --timestamp=none "${DEST}" >/dev/null 2>&1 || true
else
    /usr/bin/codesign --force --sign "${SIGN_IDENTITY}" --timestamp=none \
        --options runtime "${DEST}" >/dev/null 2>&1 || \
        /usr/bin/codesign --force --sign - --timestamp=none "${DEST}" >/dev/null 2>&1 || true
fi

"${DEST}" --version || true
