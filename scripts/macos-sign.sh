#!/bin/sh
# Signs (and optionally notarizes) the macOS universal binary. GoReleaser runs
# it as the universal_binaries post hook: scripts/macos-sign.sh <binary>.
#
# The Accessibility grant that --active needs is tied to the code signature,
# so the identifier and the Developer ID must never change between releases.
#
# Environment:
#   KEEPALIVE_SIGN=0             skip signing (CI and machines without the identity)
#   KEEPALIVE_SIGN_IDENTITY      codesign identity (default: the Developer ID below)
#   KEEPALIVE_NOTARIZE=1         notarize after signing (make release sets it)
#   KEEPALIVE_NOTARY_PROFILE     notarytool keychain profile (default: keepalive-notary)
#
# A bare Mach-O binary cannot be stapled: Gatekeeper fetches the notarization
# ticket online on first launch.
set -eu

bin=${1:?usage: macos-sign.sh <binary>}
identity=${KEEPALIVE_SIGN_IDENTITY:-Developer ID Application: Stig-Ole Gundersen (49P95434EE)}
identifier=io.github.stigoleg.keepalive
profile=${KEEPALIVE_NOTARY_PROFILE:-keepalive-notary}

log() { echo "macos-sign: $*" >&2; }

if [ "${KEEPALIVE_SIGN:-1}" = 0 ]; then
	if [ "${KEEPALIVE_NOTARIZE:-0}" = 1 ]; then
		log "KEEPALIVE_NOTARIZE=1 needs a signed binary; unset KEEPALIVE_SIGN=0"
		exit 1
	fi
	log "KEEPALIVE_SIGN=0: leaving $bin unsigned"
	exit 0
fi

if ! security find-identity -v -p codesigning | grep -qF "\"$identity\""; then
	log "no valid codesigning identity \"$identity\" in the keychain"
	log "set KEEPALIVE_SIGN=0 for an unsigned build, or KEEPALIVE_SIGN_IDENTITY"
	exit 1
fi

codesign --force --options runtime --timestamp --identifier "$identifier" --sign "$identity" "$bin"
codesign --verify --strict --verbose=2 "$bin"
log "signed $bin as $identifier"

if [ "${KEEPALIVE_NOTARIZE:-0}" != 1 ]; then
	log "KEEPALIVE_NOTARIZE is not 1: skipping notarization"
	exit 0
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/keepalive-notary.XXXXXX")
trap 'rm -rf "$work"' EXIT
zip="$work/keepalive.zip"
ditto -c -k --keepParent "$bin" "$zip"

log "submitting to the notary service with profile $profile (waits for the result)"
xcrun notarytool submit "$zip" --keychain-profile "$profile" --wait --timeout 30m \
	--output-format plist >"$work/result.plist" || true
result() { # result <key>: a value from notarytool's plist, or nothing
	v=$(/usr/libexec/PlistBuddy -c "Print :$1" "$work/result.plist" 2>/dev/null) || v=
	printf '%s' "$v"
}
status=$(result status)
id=$(result id)
if [ "$status" != Accepted ]; then
	if [ -z "$id" ]; then
		log "the submission failed (see the notarytool error above)"
		exit 1
	fi
	log "notarization finished with status ${status:-unknown} (submission $id)"
	xcrun notarytool log "$id" --keychain-profile "$profile" >&2 || true
	exit 1
fi
log "notarized (submission $id)"

# Informational: for a CLI binary spctl may still answer "not an app".
spctl --assess --type execute --verbose=2 "$bin" 2>&1 | sed 's/^/macos-sign: spctl: /' >&2 || true
