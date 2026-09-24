#!/bin/sh
# Installs awg-hs from an unpacked release archive. Run it as root:
#   sudo ./install.sh
set -eu

LABEL=io.github.spark198rus.awg-hs
DIR="/Library/Application Support/AWG-HS"
PLIST="/Library/LaunchDaemons/$LABEL.plist"
HERE=$(cd "$(dirname "$0")" && pwd)

if [ "$(id -u)" -ne 0 ]; then
	echo "Run this with sudo: sudo $0" >&2
	exit 1
fi

# Stop a running copy first; that also disconnects and restores DNS.
if [ -f "$PLIST" ]; then
	launchctl bootout system "$PLIST" 2>/dev/null || launchctl unload "$PLIST" 2>/dev/null || true
fi

mkdir -p "$DIR" /usr/local/bin
install -m 755 -o root -g wheel "$HERE/awg-hs" "$DIR/awg-hs"
install -m 755 -o root -g wheel "$HERE/uninstall.sh" "$DIR/uninstall.sh"
install -m 644 -o root -g wheel "$HERE/$LABEL.plist" "$PLIST"
# Files downloaded with a browser are quarantined; launchd won't run them.
xattr -dr com.apple.quarantine "$DIR" 2>/dev/null || true
ln -sf "$DIR/awg-hs" /usr/local/bin/awg-hs

launchctl bootstrap system "$PLIST" 2>/dev/null || launchctl load -w "$PLIST"

echo "Installed. Connect with:  awg-hs up /path/to/your.conf"
