package tunnel

import (
	"net/netip"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestKillSwitchRules(t *testing.T) {
	spec := killSwitchSpec{
		Interface: "utun3",
		Endpoints: []netip.AddrPort{
			netip.MustParseAddrPort("203.0.113.7:55424"),
			netip.MustParseAddrPort("[2001:db8::1]:51820"),
		},
		DNS: []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("1.0.0.1")},
	}
	want := `# awg-hs kill switch: only the tunnel, the VPN server, the local network
# and DHCP may send traffic. The last matching rule wins.
table <awghs_lan> const { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, 224.0.0.0/4, 255.255.255.255/32, fc00::/7, fe80::/10, ff00::/8 }
table <awghs_dns> const { 1.1.1.1, 1.0.0.1 }
block return out all flags any no state
pass out on lo0 flags any no state
pass out inet proto udp from port 68 to 255.255.255.255 port 67 no state
pass out inet6 proto udp from port 546 to ff00::/8 port 547 no state
pass out to <awghs_lan> flags any no state
block return out proto { tcp, udp } to port 53 flags any no state
pass out proto { tcp, udp } to <awghs_dns> port 53 flags any no state
pass out on utun3 flags any no state
pass out proto udp to 203.0.113.7 port 55424 no state
pass out proto udp to 2001:db8::1 port 51820 no state
`
	if got := killSwitchRules(spec); got != want {
		t.Errorf("rules:\n%s\nwant:\n%s", got, want)
	}

	// Without tunnel DNS servers, DNS is left alone (it would otherwise break).
	spec.DNS = nil
	if got := killSwitchRules(spec); strings.Contains(got, "port 53") || strings.Contains(got, "awghs_dns") {
		t.Errorf("rules without DNS mention DNS:\n%s", got)
	}
}

func TestParsePfToken(t *testing.T) {
	out := "No ALTQ support in kernel\nALTQ related functions disabled\npf enabled\nToken : 14463938713434281103\n"
	if got := parsePfToken(out); got != "14463938713434281103" {
		t.Errorf("token = %q", got)
	}
	if got := parsePfToken("pfctl: pf already enabled\n"); got != "" {
		t.Errorf("token = %q", got)
	}
}

func TestAnchorRef(t *testing.T) {
	main := `scrub-anchor "com.apple/*" all fragment reassemble
anchor "com.apple/*" all
`
	if hasAnchorRef(main) {
		t.Error("found a reference that isn't there")
	}
	withRef := withAnchorRef(main)
	want := "scrub-anchor \"com.apple/*\" all fragment reassemble\nanchor \"com.apple/*\" all\nanchor \"awg-hs\"\n"
	if withRef != want {
		t.Errorf("got %q", withRef)
	}
	if !hasAnchorRef(`anchor "awg-hs" all` + "\n" + main) {
		t.Error("missed the reference as pfctl prints it")
	}
	if hasAnchorRef(`anchor "awg-hs-other" all`) {
		t.Error("matched a different anchor")
	}
	// A reference that isn't last is moved to the end.
	if got := withAnchorRef("anchor \"awg-hs\" all\n" + main); got != want {
		t.Errorf("got %q", got)
	}
}

func TestContainsLine(t *testing.T) {
	info := "Status: Enabled for 0 days 00:03:12           Debug: Urgent\n\nState Table\n"
	if !containsLine(info, "Status: Enabled") || containsLine("Status: Disabled\n", "Status: Enabled") {
		t.Error("containsLine")
	}
}

func TestKillSwitchStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "killswitch.json")
	s := &killSwitchState{Token: "123", Spec: killSwitchSpec{
		Interface: "utun4",
		Endpoints: []netip.AddrPort{netip.MustParseAddrPort("203.0.113.7:55424")},
		DNS:       []netip.Addr{netip.MustParseAddr("1.1.1.1")},
	}}
	if err := s.save(path); err != nil {
		t.Fatal(err)
	}
	got, err := loadKillSwitchState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("got %+v", got)
	}
}
