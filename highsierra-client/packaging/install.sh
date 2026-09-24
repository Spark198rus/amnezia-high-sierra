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

# The menu bar app, if this archive has it.
if [ -d "$HERE/AWG-HS.app" ]; then
	killall AWG-HS 2>/dev/null || true
	rm -rf /Applications/AWG-HS.app
	cp -R "$HERE/AWG-HS.app" /Applications/
	chown -R root:wheel /Applications/AWG-HS.app
	xattr -dr com.apple.quarantine /Applications/AWG-HS.app 2>/dev/null || true
	# Start it for whoever is using the screen.
	CONSOLE_UID=$(stat -f %u /dev/console)
	if [ "$CONSOLE_UID" -ne 0 ]; then
		launchctl asuser "$CONSOLE_UID" /usr/bin/open /Applications/AWG-HS.app || true
	fi
	echo "Installed. Use the shield in the menu bar, or connect with:  awg-hs up /path/to/your.conf"
else
	echo "Installed. Connect with:  awg-hs up /path/to/your.conf"
fi
