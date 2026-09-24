package tunnel

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The kill switch is a pf ruleset in its own anchor, referenced from the end
// of the main ruleset. pf uses the last matching rule, so these rules decide
// what may leave the Mac. Like the AmneziaVPN macOS firewall, they only
// filter outgoing traffic and keep no state. This file has no build tag so
// the rule generation can be tested anywhere.

const pfAnchor = "awg-hs"

// killSwitchSpec is what the rules allow besides the local network and DHCP.
type killSwitchSpec struct {
	Interface string           `json:"interface"`     // the tunnel's utun interface
	Endpoints []netip.AddrPort `json:"endpoints"`     // the VPN servers
	DNS       []netip.Addr     `json:"dns,omitempty"` // if set, DNS may only go to these servers
}

// lanNets are the local-network ranges the kill switch leaves open, so
// printers, the router and AirPlay keep working. They include the multicast
// and link-local ranges that IPv6 neighbor discovery needs.
var lanNets = []string{
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
	"224.0.0.0/4", "255.255.255.255/32", "fc00::/7", "fe80::/10", "ff00::/8",
}

// killSwitchRules renders spec as pf rules for the awg-hs anchor.
func killSwitchRules(spec killSwitchSpec) string {
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }

	line("# awg-hs kill switch: only the tunnel, the VPN server, the local network")
	line("# and DHCP may send traffic. The last matching rule wins.")
	line("table <awghs_lan> const { " + strings.Join(lanNets, ", ") + " }")
	if len(spec.DNS) > 0 {
		var dns []string
		for _, a := range spec.DNS {
			dns = append(dns, a.String())
		}
		line("table <awghs_dns> const { " + strings.Join(dns, ", ") + " }")
	}

	line("block return out all flags any no state")
	line("pass out on lo0 flags any no state")
	line("pass out inet proto udp from port 68 to 255.255.255.255 port 67 no state")
	line("pass out inet6 proto udp from port 546 to ff00::/8 port 547 no state")
	line("pass out to <awghs_lan> flags any no state")
	if len(spec.DNS) > 0 {
		// Keep DNS off the local network too, apart from the tunnel's servers.
		line("block return out proto { tcp, udp } to port 53 flags any no state")
		line("pass out proto { tcp, udp } to <awghs_dns> port 53 flags any no state")
	}
	if spec.Interface != "" {
		line("pass out on " + spec.Interface + " flags any no state")
	}
	for _, ep := range spec.Endpoints {
		line("pass out proto udp to " + ep.Addr().String() + " port " + strconv.Itoa(int(ep.Port())) + " no state")
	}
	return b.String()
}

// parsePfToken extracts the reference token that `pfctl -E` prints.
func parsePfToken(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if _, tok, ok := strings.Cut(line, "Token : "); ok {
			return strings.TrimSpace(tok)
		}
	}
	return ""
}

// containsLine reports whether a line of out starts with prefix, ignoring
// leading spaces.
func containsLine(out, prefix string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}

// anchorRefLine is how the main ruleset refers to the awg-hs anchor.
const anchorRefLine = `anchor "` + pfAnchor + `"`

// hasAnchorRef reports whether `pfctl -sr` output references the anchor.
func hasAnchorRef(rules string) bool {
	for _, line := range strings.Split(rules, "\n") {
		if isAnchorRef(line) {
			return true
		}
	}
	return false
}

func isAnchorRef(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), anchorRefLine)
}

// withAnchorRef returns the main filter rules from `pfctl -sr` with the
// anchor reference moved to the end, where its rules have the last word.
func withAnchorRef(rules string) string {
	var b strings.Builder
	for _, line := range strings.Split(rules, "\n") {
		if strings.TrimSpace(line) == "" || isAnchorRef(line) {
			continue
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(anchorRefLine + "\n")
	return b.String()
}

// killSwitchState is kept on disk while the kill switch is on, so a daemon
// restarted after a crash keeps blocking and can release pf later. Like the
// pf rules themselves, it does not survive a reboot.
type killSwitchState struct {
	Token string         `json:"token"`
	Spec  killSwitchSpec `json:"spec"`
}

func loadKillSwitchState(path string) (*killSwitchState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s killSwitchState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *killSwitchState) save(path string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
