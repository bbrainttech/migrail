#!/bin/sh
set -eu

repo="bbrainttech/migrail"
base_url="https://github.com/$repo/releases"
version=""
uninstall=false
install_dir="${HOME:-}/.local/bin"

usage() {
	cat <<EOF
Install the migrail binary for this machine.

Usage: install.sh [--version vX.Y.Z] [--dir DIR] [--uninstall]

  --version    Release to install. Defaults to the latest release.
  --dir        Directory to install into. Defaults to ~/.local/bin.
  --uninstall  Remove migrail from DIR instead of installing it.
EOF
}

fail() {
	echo "migrail install: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required but was not found"
}

while [ $# -gt 0 ]; do
	case "$1" in
	--version)
		[ $# -ge 2 ] || fail "--version needs a value"
		version="$2"
		shift 2
		;;
	--dir)
		[ $# -ge 2 ] || fail "--dir needs a value"
		install_dir="$2"
		shift 2
		;;
	--uninstall)
		uninstall=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		fail "unknown argument: $1"
		;;
	esac
done

detect_os() {
	case "$(uname -s)" in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	*) fail "unsupported operating system $(uname -s). On Windows, download the zip from $base_url" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "unsupported architecture $(uname -m)" ;;
	esac
}

latest_version() {
	url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$base_url/latest")" ||
		fail "could not reach $base_url/latest"
	tag="${url##*/}"
	case "$tag" in
	v[0-9]*) echo "$tag" ;;
	*) fail "no published release found at $base_url" ;;
	esac
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "sha256sum or shasum is required to verify the download"
	fi
}

uninstall_migrail() {
	target="$install_dir/migrail"
	if [ ! -e "$target" ]; then
		found="$(command -v migrail 2>/dev/null || true)"
		case "$(readlink "$found" 2>/dev/null || true)" in
		*/Caskroom/migrail/*) fail "migrail was installed with Homebrew. Run: brew uninstall migrail" ;;
		esac
		if [ -n "$found" ]; then
			fail "migrail is not in $install_dir, but $found is on your PATH. Run again with --dir $(dirname "$found")"
		fi
		fail "migrail is not installed in $install_dir"
	fi
	rm -f "$target" || fail "could not remove $target. Check that you can write to $install_dir"
	echo "Removed $target" >&2
}

if [ "$uninstall" = true ]; then
	uninstall_migrail
	exit 0
fi

need curl
need tar
need uname

os="$(detect_os)"
arch="$(detect_arch)"

if [ -z "$version" ]; then
	version="$(latest_version)"
fi
case "$version" in
v*) ;;
*) version="v$version" ;;
esac

archive="migrail_${version#v}_${os}_${arch}.tar.gz"
download_url="$base_url/download/$version"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT INT TERM

echo "Downloading migrail $version for $os/$arch" >&2
curl -fsSL -o "$workdir/$archive" "$download_url/$archive" ||
	fail "could not download $download_url/$archive"
curl -fsSL -o "$workdir/checksums.txt" "$download_url/checksums.txt" ||
	fail "could not download $download_url/checksums.txt"

expected="$(awk -v name="$archive" '$2 == name { print $1 }' "$workdir/checksums.txt")"
[ -n "$expected" ] || fail "checksums.txt has no entry for $archive"
actual="$(sha256_of "$workdir/$archive")"
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive: expected $expected, got $actual"

tar -xzf "$workdir/$archive" -C "$workdir" migrail
mkdir -p "$install_dir"
install -m 755 "$workdir/migrail" "$install_dir/migrail" 2>/dev/null ||
	fail "could not write to $install_dir. Choose another directory with --dir"

echo "Installed migrail $version to $install_dir/migrail" >&2
case ":${PATH:-}:" in
*":$install_dir:"*) ;;
*) echo "$install_dir is not on your PATH. Add it to run migrail by name." >&2 ;;
esac
