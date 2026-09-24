//go:build darwin

package tunnel

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os/exec"
	"path/filepath"
	"strings"
)

// The system tools the tunnel drives. Absolute paths, because launchd starts
// the daemon with a minimal PATH.
const (
	ifconfigCmd    = "/sbin/ifconfig"
	routeCmd       = "/sbin/route"
	netstatCmd     = "/usr/sbin/netstat"
	scutilCmd      = "/usr/sbin/scutil"
	dscacheutilCmd = "/usr/bin/dscacheutil"
	killallCmd     = "/usr/bin/killall"
)

func run(name string, args ...string) (string, error) {
	return runInput("", name, args...)
}

func runInput(stdin, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v: %s",
			filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// runStdout is runInput, but returns only standard output. pfctl prints
// notices such as "No ALTQ support in kernel" on standard error.
func runStdout(stdin, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v: %s",
			filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func familyFlag(a netip.Addr) string {
	if a.Is4() {
		return "-inet"
	}
	return "-inet6"
}

func addAddress(ifname string, p netip.Prefix) error {
	if p.Addr().Is4() {
		_, err := run(ifconfigCmd, ifname, "inet", p.String(), p.Addr().String(), "alias")
		return err
	}
	_, err := run(ifconfigCmd, ifname, "inet6", p.String(), "alias")
	return err
}

// currentGateways returns the physical network's default gateways.
func currentGateways() (gw4, gw6 string) {
	if out, err := run(netstatCmd, "-nr", "-f", "inet"); err == nil {
		gw4 = defaultGateway(out)
	}
	if out, err := run(netstatCmd, "-nr", "-f", "inet6"); err == nil {
		gw6 = defaultGateway(out)
	}
	return gw4, gw6
}

func addInterfaceRoute(ifname string, p netip.Prefix) error {
	_, err := run(routeCmd, "-q", "-n", "add", familyFlag(p.Addr()), p.String(), "-interface", ifname)
	return err
}

func deleteInterfaceRoute(ifname string, p netip.Prefix) {
	run(routeCmd, "-q", "-n", "delete", familyFlag(p.Addr()), p.String(), "-interface", ifname)
}

// setHostRoute sends traffic for addr through gw. With no gateway it
// blackholes addr instead, so a peer's traffic can never loop into its own
// tunnel.
func setHostRoute(addr netip.Addr, gw string) error {
	deleteHostRoute(addr)
	args := []string{"-q", "-n", "add", familyFlag(addr), "-host", addr.String()}
	switch {
	case gw != "":
		args = append(args, "-gateway", gw)
	case addr.Is4():
		args = append(args, "127.0.0.1", "-blackhole")
	default:
		args = append(args, "::1", "-blackhole")
	}
	_, err := run(routeCmd, args...)
	return err
}

func deleteHostRoute(addr netip.Addr) {
	run(routeCmd, "-q", "-n", "delete", familyFlag(addr), "-host", addr.String())
}

// hostRouteOK reports whether addr is routed outside ifname the intended
// way: through a real interface, or into lo0 when blackholed.
func hostRouteOK(addr netip.Addr, ifname string, blackhole bool) bool {
	out, err := run(routeCmd, "-n", "get", familyFlag(addr), addr.String())
	if err != nil {
		return false
	}
	iface := routeGetField(out, "interface")
	if iface == "" || iface == ifname {
		return false
	}
	return (iface == "lo0") == blackhole
}

// dnsManager points every network service at the tunnel's DNS servers and
// restores the originals afterwards. Like the AmneziaVPN macOS daemon, it
// writes the Setup: keys of the dynamic store, which override DHCP-provided
// servers but are never saved to disk, so a reboot always undoes them.
type dnsManager struct {
	servers []string
	search  []string
	backups map[string]*dnsConfig // service ID -> original settings (nil: none)
}

// scutil runs commands in one scutil session.
func scutil(cmds string) (string, error) {
	return runInput("open\n"+cmds+"quit\n", scutilCmd)
}

func dnsKey(serviceID string) string {
	return "Setup:/Network/Service/" + serviceID + "/DNS"
}

func serviceIDs() ([]string, error) {
	out, err := scutil("list Setup:/Network/Service/[^/]+\n")
	if err != nil {
		return nil, err
	}
	return parseServiceIDs(out), nil
}

func showDNS(serviceID string) (*dnsConfig, error) {
	out, err := scutil("show " + dnsKey(serviceID) + "\n")
	if err != nil {
		return nil, err
	}
	cfg, ok := parseDNSDict(out)
	if !ok {
		return nil, nil
	}
	return &cfg, nil
}

// apply sets the tunnel's DNS on every service that doesn't already have it,
// backing up each service the first time. Running it again re-applies the
// setting where macOS has put the original back.
func (m *dnsManager) apply() (changed bool, err error) {
	ids, err := serviceIDs()
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, errors.New("scutil lists no network services")
	}
	ours := dnsConfig{ServerAddresses: m.servers, SearchDomains: m.search}
	var cmds strings.Builder
	for _, id := range ids {
		cur, err := showDNS(id)
		if err != nil {
			return false, err
		}
		if cur != nil && equalStrings(cur.ServerAddresses, m.servers) && equalStrings(cur.SearchDomains, m.search) {
			continue
		}
		if _, saved := m.backups[id]; !saved {
			m.backups[id] = cur
		}
		cmds.WriteString(scutilSetDNS(dnsKey(id), ours))
	}
	if cmds.Len() == 0 {
		return false, nil
	}
	if _, err := scutil(cmds.String()); err != nil {
		return true, err
	}
	flushDNSCache()
	return true, nil
}

func (m *dnsManager) restore() {
	present := map[string]bool{}
	ids, err := serviceIDs()
	for _, id := range ids {
		present[id] = true
	}
	var cmds strings.Builder
	for id, backup := range m.backups {
		if err == nil && !present[id] {
			continue // the service was deleted meanwhile
		}
		if backup == nil {
			cmds.WriteString("remove " + dnsKey(id) + "\n")
		} else {
			cmds.WriteString(scutilSetDNS(dnsKey(id), *backup))
		}
	}
	m.backups = map[string]*dnsConfig{}
	if cmds.Len() == 0 {
		return
	}
	if _, err := scutil(cmds.String()); err != nil {
		log.Printf("restoring DNS settings: %v", err)
	}
	flushDNSCache()
}

func flushDNSCache() {
	run(dscacheutilCmd, "-flushcache")
	run(killallCmd, "-HUP", "mDNSResponder")
}
