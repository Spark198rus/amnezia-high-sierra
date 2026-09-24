#!/bin/sh
# Builds dist/awg-hs-VERSION.pkg from build/awg-hs. Needs macOS (pkgbuild).
# Usage: packaging/build-pkg.sh VERSION
set -eu

VERSION=$1
cd "$(dirname "$0")/.."
LABEL=io.github.spark198rus.awg-hs

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
APP="$STAGE/Library/Application Support/AWG-HS"
mkdir -p "$APP" "$STAGE/Library/LaunchDaemons"
cp build/awg-hs packaging/uninstall.sh "$APP/"
cp "packaging/$LABEL.plist" "$STAGE/Library/LaunchDaemons/"
chmod 755 "$APP/awg-hs" "$APP/uninstall.sh" packaging/pkg-scripts/*
chmod 644 "$STAGE/Library/LaunchDaemons/$LABEL.plist"
if [ -d build/AWG-HS.app ]; then
	mkdir -p "$STAGE/Applications"
	cp -R build/AWG-HS.app "$STAGE/Applications/"
fi

# Where pkgbuild offers a choice, use the payload format older Installers read.
COMPRESSION=
if pkgbuild 2>&1 | grep -q -- --compression; then
	COMPRESSION="--compression legacy"
fi

mkdir -p dist
# shellcheck disable=SC2086
COPYFILE_DISABLE=1 pkgbuild --root "$STAGE" --scripts packaging/pkg-scripts \
	--identifier "$LABEL" --version "$VERSION" --install-location / \
	--ownership recommended $COMPRESSION "dist/awg-hs-$VERSION.pkg"
