/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2017-2025 WireGuard LLC. All Rights Reserved.
 */

package device

// Stand-ins for the Go 1.21+ min/max/clear builtins and slices.Grow, so this
// package still builds with Go 1.20, the last Go release that runs on
// macOS 10.13 High Sierra.

type ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

func maxOf[T ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// growBytes mirrors slices.Grow for byte slices.
func growBytes(s []byte, n int) []byte {
	if n < 0 {
		panic("cannot be negative")
	}
	if n -= cap(s) - len(s); n > 0 {
		s = append(s[:cap(s)], make([]byte, n)...)[:len(s)]
	}
	return s
}
