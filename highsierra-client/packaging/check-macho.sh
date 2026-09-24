#!/bin/sh
# Checks that a Mach-O binary is x86_64 only and needs no macOS newer than
# the given version. Needs macOS (lipo, otool).
# Usage: packaging/check-macho.sh BINARY 10.13
set -eu

BIN=$1
WANT=$2

ARCHS=$(lipo -archs "$BIN")
if [ "$ARCHS" != x86_64 ]; then
	echo "$BIN: architectures are '$ARCHS', want x86_64" >&2
	exit 1
fi

# The minimum is in LC_VERSION_MIN_MACOSX ("version") or LC_BUILD_VERSION ("minos").
MIN=$(otool -l "$BIN" | awk '/LC_VERSION_MIN_MACOSX|LC_BUILD_VERSION/ { found = 1 }
	found && ($1 == "version" || $1 == "minos") { print $2; exit }')
if [ "$MIN" != "$WANT" ]; then
	echo "$BIN: minimum macOS is '$MIN', want $WANT" >&2
	exit 1
fi
echo "$BIN: x86_64, minimum macOS $MIN"
