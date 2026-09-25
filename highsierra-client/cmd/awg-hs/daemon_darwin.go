//go:build darwin

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/config"
	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/control"
	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/i18n"
	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/tunnel"
)

const (
	statePath      = "/var/run/awg-hs/state.json"
	killSwitchPath = "/var/run/awg-hs/killswitch.json"

	// privateDir is readable by root only: savedPath holds the last config
	// that connected (with its private key), so "up" with no argument can
	// reconnect.
	privateDir   = "/Library/Application Support/AWG-HS/private"
	savedPath    = privateDir + "/last.json"
	settingsPath = privateDir + "/settings.json"

	adminGID = 80
)

type savedConfig struct {
	Name   string `json:"name"`
	Config string `json:"config"`
}

type settings struct {
	KillSwitch *bool `json:"killSwitch,omitempty"` // unset means on
}

func (s settings) killSwitchOn() bool { return s.KillSwitch == nil || *s.KillSwitch }

type daemon struct {
	verbose bool
	ks      *tunnel.KillSwitch

	mu       sync.Mutex
	tunnel   *tunnel.Tunnel
	name     string
	settings settings
}

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	verbose := fs.Bool("verbose", false, "log AmneziaWG debug output")
	fs.Parse(args)

	log.SetFlags(log.LstdFlags)
	if os.Geteuid() != 0 {
		return errors.New("the daemon must run as root (it is started by launchd)")
	}
	log.Printf("awg-hs %s starting", version)

	if err := tunnel.Recover(statePath); err != nil {
		log.Printf("recovering from the previous run: %v", err)
	}

	os.Remove(control.SocketPath)
	l, err := net.Listen("unix", control.SocketPath)
	if err != nil {
		return err
	}
	if err := os.Chown(control.SocketPath, 0, adminGID); err != nil {
		return err
	}
	if err := os.Chmod(control.SocketPath, 0o660); err != nil {
		return err
	}

	d := &daemon{verbose: *verbose, ks: tunnel.LoadKillSwitch(killSwitchPath)}
	if err := readJSON(settingsPath, &d.settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("reading %s: %v", settingsPath, err)
	}
	if !d.settings.killSwitchOn() {
		d.ks.Disable()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sig
		log.Printf("%v: shutting down", s)
		l.Close()
	}()

	err = control.Serve(l, d.handle, authorize)
	d.mu.Lock()
	d.down()
	d.mu.Unlock()
	os.Remove(control.SocketPath)
	return err
}

// authorize admits root and members of the admin group. The socket's mode
// already enforces this; checking the peer's credentials is a second line.
func authorize(c net.Conn) error {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return errors.New("not a unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	if cred.Uid == 0 {
		return nil
	}
	for i := 0; i < int(cred.Ngroups) && i < len(cred.Groups); i++ {
		if cred.Groups[i] == adminGID {
			return nil
		}
	}
	return errors.New("only administrators can control the tunnel")
}

func (d *daemon) handle(req control.Request) control.Response {
	d.mu.Lock()
	defer d.mu.Unlock()

	var err error
	switch req.Command {
	case control.CmdUp:
		err = d.up(req)
	case control.CmdDown:
		d.down()
	case control.CmdKillSwitch:
		if req.Enable != nil {
			err = d.setKillSwitch(*req.Enable)
		}
	case control.CmdStatus:
	default:
		err = fmt.Errorf("unknown command %q", req.Command)
	}
	if err != nil {
		return control.Response{Error: i18n.Message(req.Lang, err.Error()), Status: d.status(req.Lang)}
	}
	return control.Response{OK: true, Status: d.status(req.Lang)}
}

func (d *daemon) up(req control.Request) error {
	saved := savedConfig{Name: req.Name, Config: req.Config}
	if saved.Config == "" {
		var err error
		if saved, err = loadSaved(); err != nil {
			return errors.New("no config given and none saved from an earlier connection")
		}
	}
	cfg, err := config.Load(saved.Config)
	if err != nil {
		return err
	}

	// Look the server up first: while an old tunnel is up, its DNS works
	// even with the kill switch on.
	if err := tunnel.Resolve(cfg); err != nil {
		if d.tunnel == nil && d.ks.Active() {
			return fmt.Errorf("%w\nThe kill switch is blocking the internet, so the server's name can't be looked up.\n"+
				"Run \"awg-hs down\" to lift it, then connect again.", err)
		}
		return err
	}

	// When switching servers the kill switch stays on throughout; the new
	// tunnel's rules replace the old ones.
	blocking := d.ks.Active()
	d.closeTunnel()
	var ks *tunnel.KillSwitch
	if d.settings.killSwitchOn() {
		ks = d.ks
	}
	log.Printf("connecting %q", saved.Name)
	t, err := tunnel.Start(cfg, tunnel.Options{Verbose: d.verbose, StatePath: statePath, KillSwitch: ks})
	if err != nil {
		log.Printf("connecting %q failed: %v", saved.Name, err)
		// A first connection that fails must not leave the Mac cut off; a
		// failed switch keeps blocking, as the tunnel it replaced did.
		if !blocking {
			d.ks.Disable()
		}
		return err
	}
	if ks == nil {
		d.ks.Disable()
	}
	d.tunnel, d.name = t, saved.Name
	log.Printf("connected %q on %s", saved.Name, t.Info().Interface)
	if err := writeJSON(savedPath, saved); err != nil {
		log.Printf("saving the config: %v", err)
	}
	return nil
}

// down disconnects and lifts the kill switch: disconnecting on purpose
// means going back to the normal internet.
func (d *daemon) down() {
	d.closeTunnel()
	if d.ks.Active() {
		log.Printf("lifting the kill switch")
		d.ks.Disable()
	}
}

func (d *daemon) closeTunnel() {
	if d.tunnel == nil {
		return
	}
	log.Printf("disconnecting %q", d.name)
	d.tunnel.Close()
	d.tunnel, d.name = nil, ""
}

func (d *daemon) setKillSwitch(on bool) error {
	d.settings.KillSwitch = &on
	if err := writeJSON(settingsPath, d.settings); err != nil {
		log.Printf("saving the settings: %v", err)
	}
	log.Printf("kill switch turned %s", onOff(on))
	if on {
		if d.tunnel != nil {
			return d.tunnel.SetKillSwitch(d.ks)
		}
		return nil
	}
	if d.tunnel != nil {
		d.tunnel.SetKillSwitch(nil)
	}
	d.ks.Disable()
	return nil
}

// status describes the tunnel, with warnings in lang.
func (d *daemon) status(lang string) *control.Status {
	st := &control.Status{KillSwitch: d.settings.killSwitchOn(), Blocking: d.ks.Active()}
	if saved, err := loadSaved(); err == nil {
		st.HasSavedConfig, st.SavedName = true, saved.Name
	}
	if d.tunnel == nil {
		return st
	}
	in := d.tunnel.Info()
	st.Connected = true
	st.Name = d.name
	st.Interface = in.Interface
	st.Addresses = in.Addresses
	st.Endpoint = in.Endpoint
	st.Since = in.Since.Unix()
	if !in.LastHandshake.IsZero() {
		st.LastHandshake = in.LastHandshake.Unix()
	}
	st.RxBytes, st.TxBytes = in.RxBytes, in.TxBytes
	for _, w := range in.Warnings {
		st.Warnings = append(st.Warnings, i18n.Message(lang, w))
	}
	return st
}

func loadSaved() (savedConfig, error) {
	var s savedConfig
	err := readJSON(savedPath, &s)
	if err == nil && s.Config == "" {
		err = errors.New("empty saved config")
	}
	return s, err
}

func readJSON(path string, v interface{}) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON replaces path atomically with a root-only file.
func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
