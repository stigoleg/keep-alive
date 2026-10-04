#!/bin/sh
# Fails when man/ (written by `go run ./cmd/gen-docs`) and the
# homebrew_casks.manpages list in .goreleaser.yml disagree. A listed page that
# is missing breaks `brew install`; an extra page in man/ is either a new
# command to list or a stale page that archives and packages would ship.
# GoReleaser runs it as a before hook, right after gen-docs.
set -eu
cd "$(dirname "$0")/.."

listed=$(sed -n 's|^ *- man/\([A-Za-z0-9_-]*\.1\)$|\1|p' .goreleaser.yml | sort)
actual=$(for f in man/*.1; do [ -e "$f" ] && basename "$f"; done | sort)
[ "$listed" = "$actual" ] && exit 0

echo "check-manpages: homebrew_casks.manpages in .goreleaser.yml does not match man/" >&2
echo "  listed only: $(printf '%s\n' "$listed" | grep -vxF -e "$actual" | tr '\n' ' ')" >&2
echo "  in man/ only: $(printf '%s\n' "$actual" | grep -vxF -e "$listed" | tr '\n' ' ')" >&2
echo "  update the list, or remove stale pages (rm -r man && make docs)" >&2
exit 1
