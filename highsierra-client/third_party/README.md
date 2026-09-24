# Third-party code

## amneziawg-go

`amneziawg-go/` is [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go)
at tag `v3.1.20260814` (commit `1b86b2ae0e493e7ea93f8c1a0f0cb6735b1551f1`),
the same version the main AmneziaVPN client ships. It is MIT-licensed; see
`amneziawg-go/LICENSE`.

It is patched to build with **Go 1.20**, the last Go release whose binaries run
on macOS 10.13 High Sierra (Go 1.21 needs 10.15, Go 1.25+ needs 12). The
protocol code is unchanged. The changes, recorded in
`amneziawg-go-go1.20.patch`, are:

- `go.mod`: `go 1.20`, and `golang.org/x/crypto` v0.33.0, `x/net` v0.35.0,
  `x/sys` v0.30.0 (the newest releases that still support Go 1.20).
- `device/go120compat.go`: `maxOf`, `zeroBytes` and `growBytes` stand in for
  the Go 1.21 `max`/`clear` builtins and `slices.Grow`.
- `device/*.go`: the three `for i := range <int>` loops (Go 1.22) are written
  as classic `for` loops.
- `tun/nativeendian_*.go`: `nativeEndian` stands in for
  `binary.NativeEndian` (Go 1.21).

Go 1.22 also changed loop-variable scoping. Building the upstream code with
`-gcflags=-d=loopvar=2` flags only one loop (`device/receive.go`, the handshake
worker), and there the variable does not escape the iteration, so the old
scoping behaves the same.

These were removed because the macOS binary does not use them: `outline/`
(Outline SDK) and `tun/netstack/` (gVisor netstack), which would pull in
dependencies that need newer Go, plus the upstream tooling in `tests/`,
`.github/` and `Dockerfile`.

To move to a newer upstream tag: check it out, delete the directories above,
apply the patch (fix up any conflicts), then run `go mod tidy` with Go 1.20.
