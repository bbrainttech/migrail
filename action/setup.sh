#!/bin/sh
set -eu

fail() {
	echo "::error title=migrail::$*"
	exit 1
}

case "${RUNNER_OS:-}" in
Windows) fail "The migrail action runs on Linux and macOS runners. On Windows, download migrail.exe from the releases page." ;;
esac

if [ "${MIGRAIL_VERSION:-}" = "installed" ]; then
	command -v migrail >/dev/null 2>&1 || fail "version is set to installed, but migrail is not on PATH"
	migrail version
	exit 0
fi

version="${MIGRAIL_VERSION:-}"
if [ -z "$version" ]; then
	case "${ACTION_REF:-}" in
	v[0-9]*.[0-9]*.[0-9]*) version="$ACTION_REF" ;;
	esac
fi

cache_root="${RUNNER_TOOL_CACHE:-${RUNNER_TEMP:-/tmp}}/migrail"
install_dir="$cache_root/${version:-latest}/$(uname -m)"

if [ -n "$version" ] && [ -x "$install_dir/migrail" ]; then
	echo "Using cached migrail $version"
else
	if [ -n "$version" ]; then
		sh "$GITHUB_ACTION_PATH/install.sh" --version "$version" --dir "$install_dir"
	else
		sh "$GITHUB_ACTION_PATH/install.sh" --dir "$install_dir"
	fi
fi

echo "$install_dir" >>"$GITHUB_PATH"
"$install_dir/migrail" version
