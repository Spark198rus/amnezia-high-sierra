/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2025 WireGuard LLC. All Rights Reserved.
 */

//go:build 386 || amd64 || arm || arm64 || loong64 || mips64le || mipsle || ppc64le || riscv64 || wasm

package tun

import "encoding/binary"

// nativeEndian stands in for binary.NativeEndian (Go 1.21+).
var nativeEndian = binary.LittleEndian
