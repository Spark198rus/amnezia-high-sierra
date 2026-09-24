// Command awg-hs is a minimal AmneziaWG client for macOS 10.13 High Sierra.
//
// "awg-hs daemon" is the root service that owns the tunnel (launchd starts
// it); the other subcommands talk to it over a Unix socket.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/config"
	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/control"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `awg-hs: AmneziaWG for macOS 10.13 High Sierra

Usage:
  awg-hs up [FILE | vpn://KEY | -]   connect; with no argument, reconnect with
                                     the last config that worked
  awg-hs down                        disconnect
  awg-hs status                      show the connection
  awg-hs killswitch [on | off]       show or change the kill switch (on at first)
  awg-hs check FILE | vpn://KEY      check a config without connecting
  awg-hs convert vpn://KEY           print the AmneziaWG .conf inside a key
  awg-hs version

  awg-hs daemon [-verbose]           the background service (run by launchd)

FILE is an AmneziaWG .conf exported from the AmneziaVPN app, and KEY is a
vpn:// key for a self-hosted AmneziaWG server. "-" reads from standard input.

While connected, the kill switch blocks everything that would leave the Mac
outside the tunnel, apart from the local network. If the tunnel fails it
keeps blocking until "awg-hs up" or "awg-hs down".
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "up":
		err = cmdUp(args)
	case "down":
		err = cmdSimple(control.CmdDown)
	case "status":
		err = cmdSimple(control.CmdStatus)
	case "killswitch", "kill-switch":
		err = cmdKillSwitch(args)
	case "check":
		err = cmdCheck(args)
	case "convert":
		err = cmdConvert(args)
	case "daemon":
		err = runDaemon(args)
	case "version", "-version", "--version":
		fmt.Println("awg-hs", version)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "awg-hs: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "awg-hs:", err)
		os.Exit(1)
	}
}

// readInput returns the config text an argument refers to and a name for it.
func readInput(arg string) (text, name string, err error) {
	switch {
	case arg == "-":
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		return string(b), "stdin", err
	case config.IsKey(arg):
		return arg, "vpn key", nil
	default:
		b, err := os.ReadFile(arg)
		name := strings.TrimSuffix(filepath.Base(arg), filepath.Ext(arg))
		return string(b), name, err
	}
}

func cmdUp(args []string) error {
	req := control.Request{Command: control.CmdUp}
	switch len(args) {
	case 0:
	case 1:
		text, name, err := readInput(args[0])
		if err != nil {
			return err
		}
		// Catch config mistakes here, before bothering the daemon.
		if _, err := config.Load(text); err != nil {
			return err
		}
		req.Config, req.Name = text, name
	default:
		return errors.New("up takes one config")
	}
	fmt.Println("Connecting...")
	return call(req)
}

func cmdKillSwitch(args []string) error {
	req := control.Request{Command: control.CmdKillSwitch}
	switch {
	case len(args) == 0:
	case len(args) == 1 && (args[0] == "on" || args[0] == "off"):
		on := args[0] == "on"
		req.Enable = &on
	default:
		return errors.New("usage: awg-hs killswitch [on | off]")
	}
	resp, err := control.Call(req)
	if err != nil {
		return unreachable(err)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	st := resp.Status
	fmt.Printf("Kill switch: %s\n", onOff(st.KillSwitch))
	switch {
	case st.KillSwitch && st.Connected:
		fmt.Println("Nothing can leave this Mac outside the tunnel, apart from the local network.")
	case st.KillSwitch:
		fmt.Println("It will block traffic outside the tunnel from the next \"awg-hs up\".")
	default:
		fmt.Println("If the tunnel stops working, traffic goes out directly.")
	}
	printBlocking(st)
	return nil
}

func cmdSimple(command string) error {
	return call(control.Request{Command: command})
}

func call(req control.Request) error {
	resp, err := control.Call(req)
	if err != nil {
		return unreachable(err)
	}
	if resp.Status != nil {
		printStatus(resp.Status)
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

func unreachable(err error) error {
	return fmt.Errorf("cannot reach the awg-hs service (%v).\n"+
		"Is it installed? Only administrator accounts can use it; otherwise try sudo.", err)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// printBlocking explains a kill switch that blocks without a tunnel.
func printBlocking(st *control.Status) {
	if st.Blocking && !st.Connected {
		fmt.Println()
		fmt.Println("The kill switch is blocking all traffic outside the tunnel, because the tunnel")
		fmt.Println("is not running. Run \"awg-hs up\" to reconnect, or \"awg-hs down\" to go back")
		fmt.Println("to the normal internet.")
	}
}

func printStatus(st *control.Status) {
	if !st.Connected {
		fmt.Println("Disconnected.")
		if st.HasSavedConfig && !st.Blocking {
			fmt.Printf("Run \"awg-hs up\" to reconnect to %s.\n", st.SavedName)
		}
		printBlocking(st)
		return
	}
	fmt.Printf("Connected: %s (%s)\n", st.Name, st.Interface)
	fmt.Printf("  Server:      %s\n", st.Endpoint)
	fmt.Printf("  Addresses:   %s\n", strings.Join(st.Addresses, ", "))
	since := time.Unix(st.Since, 0)
	fmt.Printf("  Since:       %s (%s)\n", since.Format("2006-01-02 15:04:05"), ago(since))
	if st.LastHandshake != 0 {
		fmt.Printf("  Handshake:   %s ago\n", ago(time.Unix(st.LastHandshake, 0)))
	} else {
		fmt.Println("  Handshake:   none yet")
		if time.Since(since) > 15*time.Second {
			fmt.Println("               (the server is not answering: check its address and port,")
			fmt.Println("                and that the config is current)")
		}
	}
	fmt.Printf("  Traffic:     %s received, %s sent\n", bytesText(st.RxBytes), bytesText(st.TxBytes))
	fmt.Printf("  Kill switch: %s\n", onOff(st.KillSwitch))
	for _, w := range st.Warnings {
		fmt.Println("  Warning:    ", w)
	}
}

func cmdCheck(args []string) error {
	if len(args) != 1 {
		return errors.New("check takes one config")
	}
	text, _, err := readInput(args[0])
	if err != nil {
		return err
	}
	cfg, err := config.Load(text)
	if err != nil {
		return err
	}
	in := cfg.Interface
	fmt.Println("Config OK.")
	fmt.Printf("  Addresses:  %s\n", joinPrefixes(in.Addresses))
	var dns []string
	for _, a := range in.DNS {
		dns = append(dns, a.String())
	}
	fmt.Printf("  DNS:        %s\n", orNone(strings.Join(append(dns, in.DNSSearch...), ", ")))
	fmt.Printf("  MTU:        %d\n", in.MTUOrDefault())
	var awg []string
	for _, p := range in.AWG {
		awg = append(awg, p.Key)
	}
	sort.Strings(awg)
	fmt.Printf("  AmneziaWG:  %s\n", orNone(strings.Join(awg, " ")))
	for i, p := range cfg.Peers {
		fmt.Printf("  Peer %d:     %s, AllowedIPs %s\n", i+1, orNone(p.Endpoint), joinPrefixes(p.AllowedIPs))
	}
	return nil
}

func cmdConvert(args []string) error {
	if len(args) != 1 {
		return errors.New("convert takes one vpn:// key")
	}
	text, _, err := readInput(args[0])
	if err != nil {
		return err
	}
	conf, err := config.DecodeKey(text)
	if err != nil {
		return err
	}
	fmt.Print(conf)
	if !strings.HasSuffix(conf, "\n") {
		fmt.Println()
	}
	return nil
}

func joinPrefixes(ps []netip.Prefix) string {
	var s []string
	for _, p := range ps {
		s = append(s, p.String())
	}
	return orNone(strings.Join(s, ", "))
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func ago(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	if d < 0 {
		d = 0
	}
	return d.String()
}

func bytesText(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
