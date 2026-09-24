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
	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/tunnel"
)

const (
	statePath = "/var/run/awg-hs/state.json"
	// savedPath keeps the last config that connected, so "up" with no
	// argument can reconnect. It holds the private key, so only root reads it.
	savedPath = "/Library/Application Support/AWG-HS/private/last.json"
	adminGID  = 80
)

type savedConfig struct {
	Name   string `json:"name"`
	Config string `json:"config"`
}

type daemon struct {
	verbose bool

	mu     sync.Mutex
	tunnel *tunnel.Tunnel
	name   string
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

	d := &daemon{verbose: *verbose}

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
	case control.CmdStatus:
	default:
		err = fmt.Errorf("unknown command %q", req.Command)
	}
	if err != nil {
		return control.Response{Error: err.Error(), Status: d.status()}
	}
	return control.Response{OK: true, Status: d.status()}
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

	d.down()
	log.Printf("connecting %q", saved.Name)
	t, err := tunnel.Start(cfg, tunnel.Options{Verbose: d.verbose, StatePath: statePath})
	if err != nil {
		log.Printf("connecting %q failed: %v", saved.Name, err)
		return err
	}
	d.tunnel, d.name = t, saved.Name
	log.Printf("connected %q on %s", saved.Name, t.Info().Interface)
	if err := storeSaved(saved); err != nil {
		log.Printf("saving the config: %v", err)
	}
	return nil
}

func (d *daemon) down() {
	if d.tunnel == nil {
		return
	}
	log.Printf("disconnecting %q", d.name)
	d.tunnel.Close()
	d.tunnel, d.name = nil, ""
}

func (d *daemon) status() *control.Status {
	st := &control.Status{}
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
	st.Warnings = in.Warnings
	return st
}

func loadSaved() (savedConfig, error) {
	var s savedConfig
	b, err := os.ReadFile(savedPath)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	if err == nil && s.Config == "" {
		err = errors.New("empty saved config")
	}
	return s, err
}

func storeSaved(s savedConfig) error {
	if err := os.MkdirAll(filepath.Dir(savedPath), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := savedPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, savedPath)
}
