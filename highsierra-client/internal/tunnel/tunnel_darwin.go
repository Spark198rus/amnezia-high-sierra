//go:build darwin

// Package tunnel runs an AmneziaWG tunnel on macOS: it creates the utun
// interface, drives amneziawg-go in-process, and sets up addresses, routes
// and DNS the way awg-quick does, undoing it all on Close.
package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun"

	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/config"
)

// Options configures a tunnel.
type Options struct {
	Verbose    bool        // log amneziawg-go's debug output
	StatePath  string      // where to record changes for crash recovery
	KillSwitch *KillSwitch // nil to leave traffic outside the tunnel alone
}

// Tunnel is a running AmneziaWG tunnel.
type Tunnel struct {
	opts Options
	cfg  *config.Config

	mu        sync.Mutex
	dev       *device.Device
	ifname    string
	routes    []netip.Prefix // routes into the tunnel
	endpoints []netip.Addr   // endpoints routed around the tunnel
	gw4, gw6  string         // physical gateways the endpoint routes use
	dns       *dnsManager
	state     recoveryState
	since     time.Time
	warnings  []string

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// Start brings up a tunnel for cfg.
func Start(cfg *config.Config, opts Options) (_ *Tunnel, err error) {
	if err := resolveEndpoints(cfg); err != nil {
		return nil, err
	}

	t := &Tunnel{opts: opts, cfg: cfg, stop: make(chan struct{}), done: make(chan struct{})}
	defer func() {
		if err != nil {
			t.teardown()
		}
	}()

	tdev, err := tun.CreateTUN("utun", cfg.Interface.MTUOrDefault())
	if err != nil {
		return nil, fmt.Errorf("creating the utun interface: %w", err)
	}
	if t.ifname, err = tdev.Name(); err != nil {
		tdev.Close()
		return nil, err
	}
	t.state.Interface = t.ifname

	// With the kill switch, open a way for the new tunnel before it sends
	// its first handshake.
	if opts.KillSwitch != nil {
		if err := opts.KillSwitch.apply(t.killSwitchSpec()); err != nil {
			return nil, fmt.Errorf("turning on the kill switch: %w", err)
		}
	}

	level := device.LogLevelError
	if opts.Verbose {
		level = device.LogLevelVerbose
	}
	t.dev = device.NewDevice(tdev, conn.NewDefaultBind(), device.NewLogger(level, "("+t.ifname+") "))
	if err := t.dev.IpcSet(cfg.UAPI()); err != nil {
		return nil, fmt.Errorf("configuring AmneziaWG: %w", err)
	}
	if err := t.dev.Up(); err != nil {
		return nil, fmt.Errorf("starting AmneziaWG: %w", err)
	}

	for _, a := range cfg.Interface.Addresses {
		if err := addAddress(t.ifname, a); err != nil {
			return nil, err
		}
	}
	if _, err := run(ifconfigCmd, t.ifname, "up"); err != nil {
		return nil, err
	}

	// Route the server's own address around the tunnel before any traffic
	// is routed into it, so the handshake can never loop.
	t.endpoints = endpointsInTunnel(cfg)
	t.gw4, t.gw6 = currentGateways()
	for _, ep := range t.endpoints {
		t.setEndpointRoute(ep)
	}

	for _, p := range tunnelRoutes(cfg) {
		if err := addInterfaceRoute(t.ifname, p); err != nil {
			if p.Addr().Is4() {
				return nil, err
			}
			// A Mac without IPv6 may refuse IPv6 routes; carry on without them.
			log.Printf("%s: %v", t.ifname, err)
			t.warnOnce("IPv6 traffic is not going through the tunnel (could not add the IPv6 route)")
			continue
		}
		t.routes = append(t.routes, p)
	}

	if len(cfg.Interface.DNS) > 0 {
		t.dns = &dnsManager{backups: map[string]*dnsConfig{}, search: cfg.Interface.DNSSearch}
		for _, a := range cfg.Interface.DNS {
			t.dns.servers = append(t.dns.servers, a.String())
		}
		t.state.DNSBackups = t.dns.backups
		if _, err := t.dns.apply(); err != nil {
			log.Printf("setting DNS: %v", err)
			t.warnOnce("DNS could not be switched to the tunnel's servers, so lookups may bypass the tunnel")
		}
		t.saveState()
	}

	t.since = time.Now()
	go t.monitor()
	return t, nil
}

// Close takes the tunnel down and restores routes and DNS.
func (t *Tunnel) Close() {
	t.stopOnce.Do(func() { close(t.stop) })
	<-t.done
	t.mu.Lock()
	defer t.mu.Unlock()
	t.teardown()
}

// SetKillSwitch turns the kill switch on (ks) or off (nil) for this tunnel.
// Turning it off here only stops maintaining it; the caller lifts it.
func (t *Tunnel) SetKillSwitch(ks *KillSwitch) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.opts.KillSwitch = ks
	if ks == nil {
		return nil
	}
	return ks.apply(t.killSwitchSpec())
}

func (t *Tunnel) killSwitchSpec() killSwitchSpec {
	spec := killSwitchSpec{Interface: t.ifname, DNS: t.cfg.Interface.DNS}
	for _, p := range t.cfg.Peers {
		if p.EndpointAddr.IsValid() {
			spec.Endpoints = append(spec.Endpoints, p.EndpointAddr)
		}
	}
	return spec
}

// Info reports the tunnel's current state.
func (t *Tunnel) Info() Info {
	t.mu.Lock()
	defer t.mu.Unlock()
	in := Info{Interface: t.ifname, Since: t.since, Warnings: append([]string(nil), t.warnings...)}
	for _, a := range t.cfg.Interface.Addresses {
		in.Addresses = append(in.Addresses, a.String())
	}
	if t.dev != nil {
		if out, err := t.dev.IpcGet(); err == nil {
			in.applyIpcGet(out)
		}
	}
	return in
}

// teardown undoes Start; it is safe on a partly started tunnel.
func (t *Tunnel) teardown() {
	if t.dns != nil {
		t.dns.restore()
		t.dns = nil
	}
	for _, ep := range t.endpoints {
		deleteHostRoute(ep)
	}
	t.endpoints = nil
	for _, p := range t.routes {
		deleteInterfaceRoute(t.ifname, p)
	}
	t.routes = nil
	if t.dev != nil {
		t.dev.Close() // closes the utun, which removes the interface
		t.dev = nil
	}
	if err := removeState(t.opts.StatePath); err != nil {
		log.Printf("removing %s: %v", t.opts.StatePath, err)
	}
}

func (t *Tunnel) gatewayFor(a netip.Addr) string {
	if a.Is4() {
		return t.gw4
	}
	return t.gw6
}

func (t *Tunnel) setEndpointRoute(ep netip.Addr) {
	gw := t.gatewayFor(ep)
	if err := setHostRoute(ep, gw); err != nil {
		log.Printf("routing server address %s outside the tunnel: %v", ep, err)
	}
	if gw == "" {
		log.Printf("no default gateway for %s; blocking it until the network is back", ep)
	}
	t.state.EndpointRoutes = nil
	for _, e := range t.endpoints {
		t.state.EndpointRoutes = append(t.state.EndpointRoutes, e.String())
	}
	t.saveState()
}

func (t *Tunnel) saveState() {
	if err := t.state.save(t.opts.StatePath); err != nil {
		log.Printf("saving %s: %v", t.opts.StatePath, err)
	}
}

func (t *Tunnel) warnOnce(msg string) {
	for _, w := range t.warnings {
		if w == msg {
			return
		}
	}
	t.warnings = append(t.warnings, msg)
}

// monitor keeps the endpoint routes and DNS in place as the Mac moves between
// networks. It reacts to routing changes reported by `route -n monitor`, and
// also checks every 15 seconds in case an event is missed.
func (t *Tunnel) monitor() {
	defer close(t.done)

	events := make(chan struct{}, 1)
	cmd := exec.Command(routeCmd, "-n", "monitor")
	if stdout, err := cmd.StdoutPipe(); err != nil {
		log.Printf("route monitor: %v", err)
	} else if err := cmd.Start(); err != nil {
		log.Printf("route monitor: %v", err)
	} else {
		go func() {
			sc := bufio.NewScanner(stdout)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "RTM_") {
					select {
					case events <- struct{}{}:
					default:
					}
				}
			}
		}()
		defer func() {
			cmd.Process.Kill()
			cmd.Wait()
		}()
	}

	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-events:
			// Let a burst of routing changes settle first.
			select {
			case <-t.stop:
				return
			case <-time.After(time.Second):
			}
			select {
			case <-events:
			default:
			}
		case <-tick.C:
		}
		t.refresh()
	}
}

func (t *Tunnel) refresh() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dev == nil {
		return
	}

	gw4, gw6 := currentGateways()
	changed := gw4 != t.gw4 || gw6 != t.gw6
	if changed {
		log.Printf("network changed: gateway %q -> %q, IPv6 gateway %q -> %q", t.gw4, gw4, t.gw6, gw6)
	}
	t.gw4, t.gw6 = gw4, gw6
	for _, ep := range t.endpoints {
		if changed || !hostRouteOK(ep, t.ifname, t.gatewayFor(ep) == "") {
			t.setEndpointRoute(ep)
		}
	}

	if t.opts.KillSwitch != nil {
		t.opts.KillSwitch.ensure()
	}

	if t.dns != nil {
		dnsChanged, err := t.dns.apply()
		if err != nil {
			log.Printf("re-applying DNS: %v", err)
		}
		if dnsChanged {
			t.saveState()
		}
	}
}

// Recover undoes the changes recorded at statePath by a daemon that exited
// without closing its tunnel. It does nothing if there is no record.
func Recover(statePath string) error {
	s, err := loadState(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		removeState(statePath)
		return err
	}
	log.Printf("cleaning up after a tunnel on %s that was not shut down", s.Interface)
	for _, r := range s.EndpointRoutes {
		if a, err := netip.ParseAddr(r); err == nil {
			deleteHostRoute(a)
		}
	}
	if len(s.DNSBackups) > 0 {
		(&dnsManager{backups: s.DNSBackups}).restore()
	}
	return removeState(statePath)
}
