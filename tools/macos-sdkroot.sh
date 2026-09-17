#!/bin/sh
set -eu

mode="${1:-path}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
printf 'int main(void) { return 0; }\n' > "$workdir/probe.c"

links() {
	env ${1:+SDKROOT="$1"} cc "$workdir/probe.c" -lresolv -framework CoreFoundation -o "$workdir/probe" >/dev/null 2>&1
}

if links ""; then
	exit 0
fi

for sdk in $(ls -d /Library/Developer/CommandLineTools/SDKs/MacOSX[0-9]*.*.sdk /Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX[0-9]*.*.sdk 2>/dev/null | sort -t X -k 3 -V -r); do
	if links "$sdk"; then
		if [ "$mode" = "--version" ]; then
			basename "$sdk" .sdk | sed 's/^MacOSX//'
		else
			echo "$sdk"
		fi
		exit 0
	fi
done
