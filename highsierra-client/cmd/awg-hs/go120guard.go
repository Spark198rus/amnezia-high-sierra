//go:build go1.21

package main

// Go 1.21 and newer build binaries that do not run on macOS 10.13, so this
// file stops such a build on purpose. Build with Go 1.20.x (see README.md).
var _ = awgHSMustBeBuiltWithGo1_20
