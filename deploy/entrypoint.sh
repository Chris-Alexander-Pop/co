#!/bin/sh
mkdir -p /var/lib/co /var/cache/co/src /var/cache/ccache /var/cache/pacman/pkg \
	/var/cache/co/go /var/cache/co/gocache /opt/cargo/bin /opt/rustup
# Volumes are created by root. makepkg runs as builder, and a 1777 parent
# does not make a root-owned ccache/tmp or cargo registry writable.
if id builder >/dev/null 2>&1; then
	chown -R builder:builder /var/cache/ccache /opt/cargo /opt/rustup \
		/var/cache/co/go /var/cache/co/gocache
	# GnuPG no longer has a default keyserver. makepkg's recv-keys then
	# fails and the PKGBUILD signature check reports an unknown key.
	install -d -o builder -g builder /home/builder/.gnupg
	touch /home/builder/.gnupg/gpg.conf
	chown builder:builder /home/builder/.gnupg/gpg.conf
	grep -q '^keyserver ' /home/builder/.gnupg/gpg.conf || \
		printf 'keyserver hkps://keyserver.ubuntu.com\n' >> /home/builder/.gnupg/gpg.conf
fi
chmod 1777 /var/cache/co/src /var/cache/ccache /var/cache/co/go /var/cache/co/gocache
# Keep the real makepkg across restarts. Only the first boot renames it.
if [ ! -x /usr/bin/makepkg.real ]; then
	mv /usr/bin/makepkg /usr/bin/makepkg.real
fi
cp /opt/co/makepkg-with-keys /usr/bin/makepkg
chmod 755 /usr/bin/makepkg
exec /usr/local/bin/co-queue
