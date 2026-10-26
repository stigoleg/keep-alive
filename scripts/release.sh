#!/bin/sh
# Guarded local release of the tagged HEAD (make release). See RELEASING.md.
#
# 1. Refuses unless the tree is clean, HEAD carries a v* tag that origin
#    also has, the CHANGELOG.md section for the version is dated,
#    GitHub credentials can update the release, tap and bucket, and the
#    signing identity and notary profile work.
# 2. Builds a preview of the release (signed, not notarized, nothing
#    published) and prints exactly what the release will publish.
# 3. Asks for "y" on stdin, then runs `goreleaser release --clean` with
#    notarization on.
#
# Environment: GITHUB_TOKEN (defaults to gh auth token), GH_PAT (optional
# separate Homebrew/Scoop token, defaults to GITHUB_TOKEN),
# KEEPALIVE_NOTARY_PROFILE (default keepalive-notary), KEEPALIVE_SIGN_IDENTITY.
set -eu

cd "$(git rev-parse --show-toplevel)"

repo=stigoleg/keep-alive
profile=${KEEPALIVE_NOTARY_PROFILE:-keepalive-notary}
identity=${KEEPALIVE_SIGN_IDENTITY:-Developer ID Application: Stig-Ole Gundersen (49P95434EE)}

die() {
	echo "release: $*" >&2
	exit 1
}

[ "$(uname -s)" = Darwin ] || die "releases are built on macOS (signing and notarization)"
command -v goreleaser >/dev/null 2>&1 || die "goreleaser is not installed (brew install goreleaser)"
command -v gh >/dev/null 2>&1 || die "GitHub CLI is not installed (brew install gh)"

[ -z "$(git status --porcelain)" ] || die "the working tree is not clean"
tag=$(git describe --exact-match --tags --match 'v*' HEAD 2>/dev/null) ||
	die "HEAD has no v* tag (git tag -a vX.Y.Z -m 'keepalive X.Y.Z')"
version=${tag#v}
head=$(git rev-parse HEAD)

heading=$(grep -F "## [$version]" CHANGELOG.md | head -n 1 || true)
[ -n "$heading" ] || die "CHANGELOG.md has no \"## [$version]\" section"
case $heading in
*[Uu]nreleased*) die "CHANGELOG.md still says $version is Unreleased; put the release date in its heading" ;;
esac

if [ -z "${GITHUB_TOKEN:-}" ]; then
	GITHUB_TOKEN=$(gh auth token 2>/dev/null) || die "GitHub credentials are unavailable (gh auth login, or set GITHUB_TOKEN)"
fi
[ -n "$GITHUB_TOKEN" ] || die "GitHub credentials are empty (see RELEASING.md)"
GH_PAT=${GH_PAT:-$GITHUB_TOKEN}
export GITHUB_TOKEN GH_PAT

# Check repository access before building or publishing anything. A scoped
# release token may need a separate GH_PAT for the tap and bucket.
for package_repo in stigoleg/homebrew-tap stigoleg/scoop-bucket; do
	can_push=$(GH_TOKEN="$GH_PAT" gh api "repos/$package_repo" --jq '.permissions.push') ||
		die "cannot access $package_repo; GH_PAT needs Contents read/write access"
	[ "$can_push" = true ] || die "token cannot push to $package_repo; set GH_PAT with Contents read/write access"
done

security find-identity -v -p codesigning | grep -qF "\"$identity\"" ||
	die "codesigning identity not in the keychain: $identity"
xcrun notarytool history --keychain-profile "$profile" >/dev/null 2>&1 ||
	die "notary profile \"$profile\" does not work (one-time setup in RELEASING.md)"

# GoReleaser creates the GitHub release for the tag; if origin lacked it,
# GitHub would create the tag on the default branch instead of HEAD.
remote=$(git ls-remote --tags origin "refs/tags/$tag" "refs/tags/$tag^{}" |
	awk -v t="refs/tags/$tag" '$2 == t "^{}" { peeled = $1 } $2 == t { plain = $1 } END { print (peeled != "" ? peeled : plain) }')
[ -n "$remote" ] || die "origin has no tag $tag; push it first: git push origin $tag"
[ "$remote" = "$head" ] || die "origin's $tag points at $remote, not HEAD ($head)"

work=$(mktemp -d "${TMPDIR:-/tmp}/keepalive-release.XXXXXX")
trap 'rm -rf "$work"' EXIT
notes="$work/header.md"
awk -v h="$heading" '
	$0 == h { found = 1; next }
	found && /^## / { exit }
	found { print }
' CHANGELOG.md >"$notes"
grep -q '[^[:space:]]' "$notes" || die "the CHANGELOG.md section for $version is empty"

echo "release: building a preview of $tag (signed, not notarized, nothing published)..."
if ! KEEPALIVE_RELEASE_PREVIEW=1 KEEPALIVE_NOTARIZE=0 goreleaser release --snapshot --clean >"$work/preview.log" 2>&1; then
	tail -n 40 "$work/preview.log" >&2
	die "the preview build failed"
fi

echo
echo "About to publish keepalive $version from $tag ($head):"
echo
echo "  GitHub release \"keepalive $version\" on $repo with these assets:"
python3 - <<'EOF'
import json

artifacts = json.load(open("dist/artifacts.json"))
for a in sorted(artifacts, key=lambda a: a["name"]):
    if a["type"] in ("Archive", "Linux Package", "Checksum"):
        note = "  (macOS binary signed and notarized)" if "darwin" in a["name"] else ""
        print(f"    {a['name']}{note}")
EOF
echo "  Release notes: the \"$heading\" section of CHANGELOG.md, then the commit list."
echo
echo "  Homebrew: Casks/keepalive.rb pushed to stigoleg/homebrew-tap"
echo "  Scoop:    keepalive.json pushed to stigoleg/scoop-bucket"
echo
printf 'Publish? Type y to continue: '
read -r answer || answer=
[ "$answer" = y ] || die "aborted; nothing was published"

KEEPALIVE_NOTARIZE=1 KEEPALIVE_NOTARY_PROFILE="$profile" KEEPALIVE_SIGN_IDENTITY="$identity" \
	goreleaser release --clean --release-header "$notes"
