#!/bin/sh
# Builds dist/awg-hs-VERSION.pkg from build/awg-hs. Needs macOS (pkgbuild).
# Usage: packaging/build-pkg.sh VERSION
set -eu

VERSION=$1
cd "$(dirname "$0")/.."
LABEL=io.github.spark198rus.awg-hs

STAGE=$(mktemp -d)
COMPONENTS="$STAGE.components.plist"
EXPANDED="$STAGE.expanded"
trap 'rm -rf "$STAGE" "$COMPONENTS" "$EXPANDED"' EXIT
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

# Install the app in Applications even if a copy exists elsewhere. By
# default pkgbuild marks app bundles relocatable, and Installer then updates
# the other copy wherever it is instead.
pkgbuild --analyze --root "$STAGE" "$COMPONENTS"
i=0
while /usr/libexec/PlistBuddy -c "Print :$i" "$COMPONENTS" >/dev/null 2>&1; do
	/usr/libexec/PlistBuddy -c "Set :$i:BundleIsRelocatable false" "$COMPONENTS"
	i=$((i + 1))
done

mkdir -p dist
# shellcheck disable=SC2086
COPYFILE_DISABLE=1 pkgbuild --root "$STAGE" --component-plist "$COMPONENTS" \
	--scripts packaging/pkg-scripts \
	--identifier "$LABEL" --version "$VERSION" --install-location / \
	--ownership recommended $COMPRESSION "dist/awg-hs-$VERSION.pkg"

pkgutil --expand "dist/awg-hs-$VERSION.pkg" "$EXPANDED"
grep -E '<bundle |relocate' "$EXPANDED/PackageInfo" || true
if sed -n '/<relocate>/,/<\/relocate>/p' "$EXPANDED/PackageInfo" | grep -q '<bundle'; then
	echo "dist/awg-hs-$VERSION.pkg still relocates bundles" >&2
	exit 1
fi
