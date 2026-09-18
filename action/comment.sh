#!/bin/sh
set -eu

marker="<!-- migrail-report -->"
repo="$GITHUB_REPOSITORY"

warn() {
	echo "::warning title=migrail::$*"
	exit 0
}

[ -f "$COMMENT_FILE" ] || exit 0

ids="$(gh api --paginate "repos/$repo/issues/$PR_NUMBER/comments" \
	--jq ".[] | select(.body | startswith(\"$marker\")) | .id")" ||
	warn "couldn't read pull request comments. Give the job the pull-requests: write permission, or set comment: false."
existing="$(printf '%s\n' "$ids" | head -n 1)"

if [ -z "$existing" ] && [ "${FINDINGS:-0}" = "0" ]; then
	echo "No findings and no earlier migrail comment, so nothing to post."
	exit 0
fi

body="$(printf '%s\n%s' "$marker" "$(cat "$COMMENT_FILE")")"

if [ -n "$existing" ]; then
	gh api --method PATCH "repos/$repo/issues/comments/$existing" -f body="$body" >/dev/null ||
		warn "couldn't update the pull request comment. Give the job the pull-requests: write permission, or set comment: false."
	echo "Updated the migrail comment on pull request #$PR_NUMBER."
else
	gh api --method POST "repos/$repo/issues/$PR_NUMBER/comments" -f body="$body" >/dev/null ||
		warn "couldn't comment on the pull request. Give the job the pull-requests: write permission, or set comment: false."
	echo "Commented on pull request #$PR_NUMBER."
fi
