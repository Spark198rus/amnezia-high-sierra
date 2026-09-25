// Package control is the protocol between the awg-hs daemon, which runs as
// root, and its clients (the command-line tool, and later the menu-bar app).
// Each connection to the Unix socket carries one JSON request line and one
// JSON response line.
package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// SocketPath is where the daemon listens. It is owned by root:admin with mode
// 0660, so any administrator can control the tunnel without sudo.
const SocketPath = "/var/run/awg-hs.sock"

// Commands.
const (
	CmdUp         = "up"
	CmdDown       = "down"
	CmdStatus     = "status"
	CmdKillSwitch = "killswitch" // Enable set: turn on or off; unset: report
)

// Request is sent by a client.
type Request struct {
	Command string `json:"command"`
	// Config is .conf text or a vpn:// key for CmdUp. Empty means reconnect
	// with the last config that connected.
	Config string `json:"config,omitempty"`
	Name   string `json:"name,omitempty"` // a label for the connection
	Enable *bool  `json:"enable,omitempty"`
	// Lang is the language for messages in the reply: "ru" or "en".
	Lang string `json:"lang,omitempty"`
}

// Response is the daemon's answer. Status is set for every successful
// request.
type Response struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
}

// Status describes the tunnel. Times are Unix seconds, 0 when unset.
type Status struct {
	Connected      bool     `json:"connected"`
	Name           string   `json:"name,omitempty"`
	Interface      string   `json:"interface,omitempty"`
	Addresses      []string `json:"addresses,omitempty"`
	Endpoint       string   `json:"endpoint,omitempty"`
	Since          int64    `json:"since,omitempty"`
	LastHandshake  int64    `json:"lastHandshake,omitempty"`
	RxBytes        uint64   `json:"rxBytes,omitempty"`
	TxBytes        uint64   `json:"txBytes,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	HasSavedConfig bool     `json:"hasSavedConfig,omitempty"`
	SavedName      string   `json:"savedName,omitempty"`
	// KillSwitch is the setting; Blocking is whether its rules are in force,
	// which they stay after the tunnel fails until "down".
	KillSwitch bool `json:"killSwitch"`
	Blocking   bool `json:"blocking,omitempty"`
}

const maxMessage = 1 << 20

// Call sends one request to the daemon.
func Call(req Request) (*Response, error) {
	return CallAt(SocketPath, req)
}

// CallAt sends one request to a daemon listening at path.
func CallAt(path string, req Request) (*Response, error) {
	c, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Minute))

	if err := json.NewEncoder(c).Encode(req); err != nil {
		return nil, err
	}
	var resp Response
	if err := json.NewDecoder(io.LimitReader(c, maxMessage)).Decode(&resp); err != nil {
		return nil, fmt.Errorf("reading the daemon's reply: %w", err)
	}
	return &resp, nil
}

// Serve answers requests on l until l is closed. authorize, if set, vets
// each connection before its request is read.
func Serve(l net.Listener, handle func(Request) Response, authorize func(net.Conn) error) error {
	for {
		c, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go serveConn(c, handle, authorize)
	}
}

func serveConn(c net.Conn, handle func(Request) Response, authorize func(net.Conn) error) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Minute))

	var resp Response
	if authorize != nil {
		if err := authorize(c); err != nil {
			resp.Error = err.Error()
			json.NewEncoder(c).Encode(resp)
			return
		}
	}
	line, err := bufio.NewReader(io.LimitReader(c, maxMessage)).ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		resp.Error = "malformed request"
	} else {
		resp = handle(req)
	}
	if err := json.NewEncoder(c).Encode(resp); err != nil {
		log.Printf("control: writing reply: %v", err)
	}
}
