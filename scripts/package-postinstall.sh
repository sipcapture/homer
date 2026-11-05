#!/bin/sh
# homer-core maintainer script (deb postinst and rpm %post).
# Creates the system user the service runs as, and hands it the data
# directories. The binary, unit, and config stay root-owned.
set -eu

case "${1:-}" in
	abort-upgrade|abort-remove|abort-deconfigure)
		exit 0
		;;
esac

# A system uid comes from the OS. Do not pin 1000: that id is often the
# first human user. The Docker image uses uid 1000 on its own.
if ! getent group homer >/dev/null 2>&1; then
	groupadd --system homer
fi
if ! getent passwd homer >/dev/null 2>&1; then
	shell=/usr/sbin/nologin
	if [ ! -x "$shell" ]; then
		shell=/sbin/nologin
	fi
	useradd --system --gid homer --home-dir /var/lib/homer --shell "$shell" --comment "Homer Core" homer
fi

# /data itself stays root. Only the leaf directories are handed to homer.
mkdir -p /var/lib/homer /var/log/homer /data/homer /run/homer-core /usr/local/homer-core/.duckdb

# First upgrade walks these trees once. Parquet created as root mode 0600
# would otherwise be unreadable after the process drops privileges.
chown_service_tree() {
	dir=$1
	[ -d "$dir" ] || return 0
	chown -R homer:homer "$dir"
}

chown_service_tree /var/lib/homer
chown_service_tree /var/log/homer
chown_service_tree /run/homer-core
chown_service_tree /usr/local/homer-core/.duckdb

# Take /data/homer only when root still owns the whole tree (the previous
# package ran as root). A tree that already contains another uid, such as
# the Docker image's uid 1000, is left alone.
if [ "$(stat -c '%u' /data/homer)" = "0" ]; then
	foreign=$(find /data/homer ! -user root -print -quit)
	if [ -n "$foreign" ]; then
		echo "homer-core: leaving /data/homer unchanged; $foreign is not owned by root" >&2
	else
		chown_service_tree /data/homer
	fi
fi

chmod 0750 /var/lib/homer /var/log/homer /run/homer-core
if [ "$(stat -c '%u' /data/homer)" = "$(id -u homer)" ]; then
	chmod 0750 /data/homer
fi

# The live config is not shipped; the example is homer.json.example.
# A root-owned config must be group-readable or the service cannot open it.
cfg=/usr/local/homer-core/etc/homer.json
if [ -f "$cfg" ] && [ "$(stat -c '%u' "$cfg")" = "0" ]; then
	chgrp homer "$cfg"
	chmod 0640 "$cfg"
fi

# DuckDB's azure extension only looks at the RedHat CA bundle path
# (duckdb/duckdb-azure#185). The process can no longer create this as root.
if [ ! -e /etc/pki/tls/certs/ca-bundle.crt ] && [ -f /etc/ssl/certs/ca-certificates.crt ]; then
	mkdir -p /etc/pki/tls/certs
	ln -sf /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt
fi

if command -v systemd-tmpfiles >/dev/null 2>&1; then
	systemd-tmpfiles --create /usr/lib/tmpfiles.d/homer-core.conf || true
fi

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload
	systemctl try-restart homer-core
elif [ -x /etc/init.d/homer-core ] && [ -f /var/run/homer-core.pid ]; then
	/etc/init.d/homer-core restart
fi
