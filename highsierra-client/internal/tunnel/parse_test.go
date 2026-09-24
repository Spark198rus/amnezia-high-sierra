package tunnel

import (
	"net/netip"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/config"
)

func TestDefaultGateway(t *testing.T) {
	inet := `Routing tables

Internet:
Destination        Gateway            Flags        Refs      Use   Netif Expire
default            link#14            UCSI            1        0   utun1
default            192.168.1.1        UGSc          104        0     en0
0/1                utun3              USc             3        0   utun3
127                127.0.0.1          UCS             0        0     lo0
`
	if gw := defaultGateway(inet); gw != "192.168.1.1" {
		t.Errorf("IPv4 gateway = %q", gw)
	}
	inet6 := `Routing tables

Internet6:
Destination                             Gateway                         Flags         Netif Expire
default                                 fe80::1%en0                     UGc             en0
::1                                     ::1                             UHL             lo0
`
	if gw := defaultGateway(inet6); gw != "fe80::1%en0" {
		t.Errorf("IPv6 gateway = %q", gw)
	}
	if gw := defaultGateway("Internet:\nDestination Gateway Flags\n127 127.0.0.1 UCS 0 0 lo0\n"); gw != "" {
		t.Errorf("gateway without a default route = %q", gw)
	}
}

func TestRouteGetField(t *testing.T) {
	out := `   route to: 203.0.113.7
destination: 203.0.113.7
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,HOST,DONE,STATIC>
 recvpipe  sendpipe  ssthresh  rtt,msec    rttvar  hopcount      mtu     expire
       0         0         0         0         0         0      1500         0
`
	if got := routeGetField(out, "interface"); got != "en0" {
		t.Errorf("interface = %q", got)
	}
	if got := routeGetField(out, "gateway"); got != "192.168.1.1" {
		t.Errorf("gateway = %q", got)
	}
}

func TestParseDNSDict(t *testing.T) {
	out := `<dictionary> {
  DomainName : home
  SearchDomains : <array> {
    0 : home
    1 : example.com
  }
  ServerAddresses : <array> {
    0 : 192.168.1.1
    1 : fe80::1%en0
  }
}
`
	cfg, ok := parseDNSDict(out)
	want := dnsConfig{
		ServerAddresses: []string{"192.168.1.1", "fe80::1%en0"},
		SearchDomains:   []string{"home", "example.com"},
		DomainName:      "home",
	}
	if !ok || !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v, %v", cfg, ok)
	}
	if _, ok := parseDNSDict("  No such key\n"); ok {
		t.Error("missing key parsed as present")
	}
}

func TestParseServiceIDs(t *testing.T) {
	out := `  subKey [0] = Setup:/Network/Service/0B4F4F1E-1234-4C3A-9E2D-6C2D3B1A0F11
  subKey [1] = Setup:/Network/Service/0B4F4F1E-1234-4C3A-9E2D-6C2D3B1A0F11/DNS
  subKey [2] = Setup:/Network/Service/A2C81D55-99F0-4A8B-8C1E-222222222222/IPv4
  subKey [3] = State:/Network/Service/FFFFFFFF-0000-0000-0000-000000000000/DNS
`
	want := []string{"0B4F4F1E-1234-4C3A-9E2D-6C2D3B1A0F11", "A2C81D55-99F0-4A8B-8C1E-222222222222"}
	if got := parseServiceIDs(out); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestScutilSetDNS(t *testing.T) {
	got := scutilSetDNS("Setup:/Network/Service/X/DNS", dnsConfig{
		ServerAddresses: []string{"1.1.1.1", "1.0.0.1"},
		DomainName:      "home",
	})
	want := "d.init\nd.add ServerAddresses * 1.1.1.1 1.0.0.1\nd.add DomainName home\nset Setup:/Network/Service/X/DNS\n"
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestApplyIpcGet(t *testing.T) {
	out := "private_key=00\nlisten_port=51820\n" +
		"public_key=aa\nendpoint=203.0.113.7:55424\nlast_handshake_time_sec=1700000000\nlast_handshake_time_nsec=5\nrx_bytes=100\ntx_bytes=200\n" +
		"public_key=bb\nlast_handshake_time_sec=1700000100\nlast_handshake_time_nsec=0\nrx_bytes=1\ntx_bytes=2\n" +
		"errno=0\n"
	var in Info
	in.applyIpcGet(out)
	if in.Endpoint != "203.0.113.7:55424" || in.RxBytes != 101 || in.TxBytes != 202 ||
		!in.LastHandshake.Equal(time.Unix(1700000100, 0)) {
		t.Errorf("got %+v", in)
	}
}

func TestRoutesAndEndpoints(t *testing.T) {
	cfg := &config.Config{Peers: []config.Peer{{
		AllowedIPs: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/0"),
			netip.MustParsePrefix("::/0"),
			netip.MustParsePrefix("10.1.2.3/8"),
			netip.MustParsePrefix("10.0.0.0/8"),
		},
		EndpointAddr: netip.MustParseAddrPort("203.0.113.7:55424"),
	}}}
	wantRoutes := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/1"),
		netip.MustParsePrefix("128.0.0.0/1"),
		netip.MustParsePrefix("::/1"),
		netip.MustParsePrefix("8000::/1"),
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	if got := tunnelRoutes(cfg); !reflect.DeepEqual(got, wantRoutes) {
		t.Errorf("routes = %v", got)
	}
	if got := endpointsInTunnel(cfg); len(got) != 1 || got[0] != netip.MustParseAddr("203.0.113.7") {
		t.Errorf("endpoints = %v", got)
	}

	cfg.Peers[0].AllowedIPs = []netip.Prefix{netip.MustParsePrefix("10.8.1.0/24")}
	if got := endpointsInTunnel(cfg); len(got) != 0 {
		t.Errorf("split tunnel endpoints = %v", got)
	}
}

func TestResolveLiteral(t *testing.T) {
	cfg := &config.Config{Peers: []config.Peer{{Endpoint: "[2001:db8::1]:51820"}, {Endpoint: "198.51.100.4:55424"}}}
	if err := resolveEndpoints(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Peers[0].EndpointAddr.String() != "[2001:db8::1]:51820" || cfg.Peers[1].EndpointAddr.String() != "198.51.100.4:55424" {
		t.Errorf("got %v %v", cfg.Peers[0].EndpointAddr, cfg.Peers[1].EndpointAddr)
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "state.json")
	s := &recoveryState{
		Interface:      "utun3",
		EndpointRoutes: []string{"203.0.113.7"},
		DNSBackups: map[string]*dnsConfig{
			"A": {ServerAddresses: []string{"192.168.1.1"}},
			"B": nil,
		},
	}
	if err := s.save(path); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("got %+v", got)
	}
	if b, ok := got.DNSBackups["B"]; !ok || b != nil {
		t.Error("a service without DNS settings must survive as a nil backup")
	}
	if err := removeState(path); err != nil {
		t.Fatal(err)
	}
	if err := removeState(path); err != nil {
		t.Errorf("removing a missing state file: %v", err)
	}
}
