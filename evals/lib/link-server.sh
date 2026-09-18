# Sourced by each case's scaffold.sh.
#
# Every run gets its own HOME, and bin/evaluate-launch resolves both the binary
# and the key file under $HOME. Without these links the server exits before the
# client speaks to it, and the run reads as "the model chose not to use the
# tool" when nothing was ever offered.
#
# Symlinks, not copies: the key stays in one place on disk, and readKeyFile's
# permission check follows to the real file's own mode.
link_evaluate_server() {
	# The operator's real home, which HOME no longer points at. `id -un` is the
	# account running the eval; deriving it from HOME would be circular.
	real_home=$(eval echo "~$(id -un)")
	for pair in \
		".local/libexec/racecraft-jev/evaluate" \
		".config/racecraft-jev/openrouter.key"; do
		[ -e "$real_home/$pair" ] || continue
		mkdir -p "$(dirname "$HOME/$pair")"
		ln -sf "$real_home/$pair" "$HOME/$pair"
	done
}
