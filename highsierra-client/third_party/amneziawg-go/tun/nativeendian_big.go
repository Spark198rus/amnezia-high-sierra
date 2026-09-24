/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2025 WireGuard LLC. All Rights Reserved.
 */

//go:build armbe || arm64be || m68k || mips || mips64 || mips64p32 || ppc || ppc64 || s390 || s390x || shbe || sparc || sparc64

package tun

import "encoding/binary"

// nativeEndian stands in for binary.NativeEndian (Go 1.21+).
var nativeEndian = binary.BigEndian
