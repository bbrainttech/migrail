#!/bin/sh
set -u

out_dir="${RUNNER_TEMP:-/tmp}/migrail-action"
mkdir -p "$out_dir"
report="$out_dir/report.json"

set -- check --fail-on "$MIGRAIL_FAIL_ON" -o "json=$report"

if [ -n "${MIGRAIL_DB_VERSION:-}" ]; then
	set -- "$@" --db-version "$MIGRAIL_DB_VERSION"
fi

if [ "${MIGRAIL_SARIF:-false}" = "true" ]; then
	set -- "$@" -o "sarif=$out_dir/migrail.sarif"
	echo "sarif-file=$out_dir/migrail.sarif" >>"$GITHUB_OUTPUT"
fi

if [ "${MIGRAIL_COMMENT:-false}" = "true" ]; then
	set -- "$@" -o "markdown=$out_dir/comment.md"
	echo "comment-file=$out_dir/comment.md" >>"$GITHUB_OUTPUT"
fi

# shellcheck disable=SC2086
migrail "$@" ${MIGRAIL_ARGS:-}
code=$?

echo "exit-code=$code" >>"$GITHUB_OUTPUT"

if [ -f "$report" ] && command -v jq >/dev/null 2>&1; then
	findings="$(jq '.summary.errors + .summary.warnings + .summary.notices' "$report")"
	echo "findings=$findings" >>"$GITHUB_OUTPUT"
fi

exit 0
