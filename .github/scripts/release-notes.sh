#!/bin/sh
# Prints the GitHub release notes of a tag: its section of CHANGELOG.md,
# then its section of CHANGELOG.ja.md under a 日本語 heading. A section
# starts at the line "## <tag>" and ends before the next "## " line; the
# heading itself is left out, since the release is titled with the tag.
# Exits 1 when CHANGELOG.md has no section for the tag.
#
#	.github/scripts/release-notes.sh v0.0.1-beta.3
set -eu
tag=$1
cd "$(dirname "$0")/../.."

section() {
	awk -v h="## $tag" '
		$0 == h { on = 1; next }
		on && /^## / { exit }
		on { print }
	' "$1" | sed -e '/./,$!d'
}

en=$(section CHANGELOG.md)
[ -n "$en" ] || { echo "CHANGELOG.md has no section for $tag" >&2; exit 1; }
printf '%s\n' "$en"
ja=$(section CHANGELOG.ja.md)
if [ -n "$ja" ]; then
	printf '\n## 日本語\n\n%s\n' "$ja"
fi
