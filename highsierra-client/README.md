# awg-hs: AmneziaWG for macOS 10.13 High Sierra

A small AmneziaWG-only client for Macs that the AmneziaVPN app no longer
supports. The AmneziaVPN app needs macOS 12 or newer; this client targets
macOS 10.13 on Intel Macs.

It has two parts in one program, `awg-hs`:

- **A background service** (`awg-hs daemon`), started by launchd as root. It
  creates the tunnel interface, runs AmneziaWG, sets up routes and DNS, and
  restores everything when you disconnect.
- **A command-line tool** (`awg-hs up`, `down`, `status`), which asks the
  service to connect or disconnect. Any administrator account can use it
  without `sudo`.

A menu-bar app is planned next; it will use the same service.

> **Status:** version 0.1 compiles, and the parts that can be tested off a Mac
> are tested, but it has **not yet been run on a real High Sierra Mac**.

## What it supports

- AmneziaWG connections, with the same AmneziaWG version as the current
  AmneziaVPN app (all of `Jc`/`Jmin`/`Jmax`, `S1`–`S4`, `H1`–`H4`, `I1`–`I5`
  and the newer settings). Plain WireGuard configs work too.
- Configs as a `.conf` file, or as a `vpn://` key for a **self-hosted** server.
  Amnezia Premium and free subscription keys don't work: they go through
  Amnezia's own service.
- Full tunnel (`AllowedIPs = 0.0.0.0/0, ::/0`) and split tunnel (only the
  listed networks).
- Moving between networks: the route to the server and the DNS settings are
  kept up to date as the Mac changes Wi-Fi networks or wakes from sleep.

Not supported yet:

- **Kill switch.** If the tunnel drops, traffic goes out directly.
- **IPv6 leak blocking.** If macOS refuses the IPv6 routes, `awg-hs status`
  shows a warning, and IPv6 traffic bypasses the tunnel.
- **Connecting automatically at startup.** After a restart, run `awg-hs up`.

## Getting a config

In the AmneziaVPN app on another device, share the connection to your server:

- save it in **AmneziaWG format** to get a `.conf` file, or
- copy the **`vpn://` key**.

## Installing

Download `awg-hs-VERSION-macos-x86_64.tar.gz` (or the `.pkg`) from the
artifacts of the latest "High Sierra client" run on the repository's Actions
tab. (On a fork, Actions must be enabled first.) Then in Terminal:

```sh
tar xzf awg-hs-0.1.0-macos-x86_64.tar.gz
cd awg-hs-0.1.0-macos-x86_64
sudo ./install.sh
```

This installs the program to `/Library/Application Support/AWG-HS/`, links it
to `/usr/local/bin/awg-hs`, and starts the service. To use the `.pkg` instead,
right-click it and choose **Open**, since it isn't signed.

## Using it

```sh
awg-hs up ~/Downloads/my-server.conf     # connect with a .conf file
awg-hs up 'vpn://AAAA...'                # or with a key (keep the quotes)
awg-hs status                            # check the handshake and traffic
awg-hs down                              # disconnect
awg-hs up                                # reconnect with the last config
```

`awg-hs check FILE` validates a config without connecting, and
`awg-hs convert 'vpn://...' > my.conf` saves the config inside a key as a file.

If `status` shows **Handshake: none yet**, the server isn't answering. Check
that the server is running and that the config is current.

## Troubleshooting

- The service logs to `/Library/Logs/awg-hs.log`.
- For detailed AmneziaWG logging, add `<string>-verbose</string>` after
  `<string>daemon</string>` in
  `/Library/LaunchDaemons/io.github.spark198rus.awg-hs.plist`, then restart
  the service:

  ```sh
  sudo launchctl bootout system /Library/LaunchDaemons/io.github.spark198rus.awg-hs.plist
  sudo launchctl bootstrap system /Library/LaunchDaemons/io.github.spark198rus.awg-hs.plist
  ```

  Reinstalling puts the original file back.
- If the service stops unexpectedly, launchd restarts it. The restarted
  service first undoes the routes and DNS changes the old one made.
- All DNS changes live only in memory, so a restart always clears them.

## Uninstalling

```sh
sudo "/Library/Application Support/AWG-HS/uninstall.sh"
```

## Building

This needs **Go 1.20.x** (for example 1.20.14). Go 1.21 and newer build
binaries that won't run on macOS 10.13, so the build refuses them. No cgo is
involved, so you can build on Linux as well as on a Mac.

```sh
make test     # vet and unit tests
make build    # build/awg-hs, checked to be x86_64 and to need only macOS 10.13
make dist     # dist/awg-hs-VERSION-macos-x86_64.tar.gz
make pkg      # dist/awg-hs-VERSION.pkg (needs macOS)
```

The GitHub Actions workflow `.github/workflows/highsierra-client.yml` runs
these steps on every push that touches this folder.

## How it works

- `third_party/amneziawg-go`: the AmneziaWG implementation, patched to build
  with Go 1.20 (see `third_party/README.md`). The service runs it in-process.
- `internal/config`: reads `.conf` files and `vpn://` keys and turns them into
  AmneziaWG settings.
- `internal/tunnel`: creates the `utun` interface and sets addresses, routes
  and DNS with `ifconfig`, `route` and `scutil`, the same way `awg-quick` and
  the AmneziaVPN macOS service do. A full-tunnel route is added as two halves
  (`0.0.0.0/1` and `128.0.0.0/1`) so the normal default route stays in place.
  The server's own address gets a route outside the tunnel.
- `internal/control`: the JSON protocol on `/var/run/awg-hs.sock` between the
  command-line tool and the service.
- `cmd/awg-hs`: the program itself.
