#!/bin/sh
# Install the latest Racecraft evaluate release from GitHub.
#
# Fetch this script, read it, then run it:
#   curl -fsSL -o install.sh \
#     https://raw.githubusercontent.com/racecraft-lab/typesafe-mcp/main/install.sh
#   sh install.sh
#
# Override the target directory with EVALUATE_INSTALL_DIR.
#
# This is Racecraft Lab's fork. It installs into its own directory rather than
# ~/.local/bin, so it cannot overwrite an upstream `evaluate` already on PATH,
# and it never replaces an existing file unless EVALUATE_FORCE=1.
#
# This repository has moved. evaluate now lives in
# racecraft-lab/racecraft-plugins-public as typesafe-jev, which releases more
# than one component, so its "latest" release can belong to another one. This
# script therefore installs this repository's last release, the bridge, and
# then runs that binary's `evaluate update`: it lists the new repository's
# releases, picks the newest typesafe-jev-v* tag, and checks it against that
# release's SHA256SUMS.txt. Parsing the release list here in sh would be a
# second copy of that logic.
set -eu

REPO="racecraft-lab/typesafe-mcp"
INSTALL_DIR="${EVALUATE_INSTALL_DIR:-$HOME/.local/libexec/racecraft-jev}"
FORCE="${EVALUATE_FORCE:-0}"

main() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	machine=$(uname -m)
	case "$machine" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) arch= ;;
	esac
	if { [ "$os" != darwin ] && [ "$os" != linux ]; } || [ -z "$arch" ]; then
		echo "unsupported platform: $os $machine (supported: darwin/linux on amd64/arm64)" >&2
		exit 1
	fi

	archive="evaluate-$os-$arch.tar.gz"
	base="https://github.com/$REPO/releases/latest/download"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	echo "Downloading $archive..."
	curl -fsSL -o "$tmp/$archive" "$base/$archive"
	curl -fsSL -o "$tmp/SHA256SUMS.txt" "$base/SHA256SUMS.txt"

	if command -v sha256sum >/dev/null 2>&1; then
		got=$(sha256sum "$tmp/$archive" | awk '{print $1}')
	else
		got=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
	fi
	want=$(awk -v f="$archive" '$2 == f {print $1}' "$tmp/SHA256SUMS.txt")
	if [ -z "$want" ]; then
		echo "no checksum for $archive in SHA256SUMS.txt" >&2
		exit 1
	fi
	if [ "$got" != "$want" ]; then
		echo "checksum mismatch: got $got, want $want" >&2
		exit 1
	fi

	target="$INSTALL_DIR/evaluate"
	if [ -e "$target" ] && [ "$FORCE" != 1 ]; then
		echo "$target already exists." >&2
		echo "Keep a copy of it first, then re-run with EVALUATE_FORCE=1 to replace it." >&2
		exit 1
	fi

	tar -xzf "$tmp/$archive" -C "$tmp" evaluate
	chmod +x "$tmp/evaluate"
	mkdir -p "$INSTALL_DIR"
	# Staged inside the target directory so the rename is atomic and never
	# crosses a filesystem: an interrupted install leaves the old binary intact.
	staged="$INSTALL_DIR/.evaluate.$$"
	mv "$tmp/evaluate" "$staged"
	mv "$staged" "$target"

	echo "Installed to $target"

	echo "Moving to the newest typesafe-jev release from racecraft-lab/racecraft-plugins-public..."
	if ! "$target" update; then
		echo "evaluate update failed; $target still holds this repository's last release." >&2
		echo "Re-run it with: $target update" >&2
		exit 1
	fi

	# Deliberately not added to PATH: an isolated absolute path cannot shadow,
	# or be shadowed by, another evaluate install. Point clients at it directly.
	echo "Register it with: $target setup mcp --client claude-code --client codex" >&2
	"$target" version --verbose
}

main "$@"
