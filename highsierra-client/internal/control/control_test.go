package control

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awg-hs.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	requests := make(chan Request, 1)
	go Serve(l, func(req Request) Response {
		requests <- req
		return Response{OK: true, Status: &Status{Connected: true, Name: req.Name}}
	}, nil)

	resp, err := CallAt(path, Request{Command: CmdUp, Config: "[Interface]\n", Name: "home"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Status == nil || resp.Status.Name != "home" {
		t.Errorf("response = %+v", resp)
	}
	if got := <-requests; got.Command != CmdUp || got.Config != "[Interface]\n" {
		t.Errorf("request = %+v", got)
	}
}

func TestUnauthorized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "awg-hs.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	called := make(chan bool, 1)
	go Serve(l, func(Request) Response { called <- true; return Response{OK: true} },
		func(net.Conn) error { return errors.New("only administrators can control the tunnel") })

	resp, err := CallAt(path, Request{Command: CmdDown})
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Error == "" || len(called) != 0 {
		t.Errorf("response = %+v, handler called = %v", resp, len(called) != 0)
	}
}
