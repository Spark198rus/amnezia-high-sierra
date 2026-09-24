#!/bin/sh
# Removes awg-hs. Run it as root:
#   sudo "/Library/Application Support/AWG-HS/uninstall.sh"
set -eu

LABEL=io.github.spark198rus.awg-hs
DIR="/Library/Application Support/AWG-HS"
PLIST="/Library/LaunchDaemons/$LABEL.plist"

if [ "$(id -u)" -ne 0 ]; then
	echo "Run this with sudo: sudo \"$0\"" >&2
	exit 1
fi

# Stopping the service disconnects the tunnel, restores DNS and lifts the
# kill switch.
if [ -f "$PLIST" ]; then
	launchctl bootout system "$PLIST" 2>/dev/null || launchctl unload "$PLIST" 2>/dev/null || true
fi

# Lift the kill switch here too, in case the service wasn't running to do it.
KS=/var/run/awg-hs/killswitch.json
if [ -f "$KS" ]; then
	pfctl -a awg-hs -F rules 2>/dev/null || true
	pfctl -a awg-hs -F Tables 2>/dev/null || true
	TOKEN=$(sed -n 's/.*"token":"\([0-9]*\)".*/\1/p' "$KS")
	if [ -n "$TOKEN" ]; then
		pfctl -X "$TOKEN" 2>/dev/null || true
	fi
fi

rm -f "$PLIST" /var/run/awg-hs.sock
if [ "$(readlink /usr/local/bin/awg-hs 2>/dev/null)" = "$DIR/awg-hs" ]; then
	rm -f /usr/local/bin/awg-hs
fi
rm -rf "$DIR" /var/run/awg-hs
pkgutil --forget "$LABEL" >/dev/null 2>&1 || true

echo "awg-hs removed. Its log is /Library/Logs/awg-hs.log if you want to delete it too."
