package tunnel

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/spark198rus/amnezia-high-sierra/highsierra-client/internal/config"
)

// Info describes a running tunnel.
type Info struct {
	Interface     string
	Addresses     []string
	Endpoint      string
	Since         time.Time
	LastHandshake time.Time // zero until the first handshake
	RxBytes       uint64
	TxBytes       uint64
	Warnings      []string
}

// applyIpcGet fills in the peer statistics from amneziawg-go's UAPI "get"
// output, summing over peers and keeping the most recent handshake.
func (in *Info) applyIpcGet(out string) {
	var sec, nsec int64
	flush := func() {
		if sec > 0 {
			if t := time.Unix(sec, nsec); t.After(in.LastHandshake) {
				in.LastHandshake = t
			}
		}
		sec, nsec = 0, 0
	}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			flush()
		case "endpoint":
			if in.Endpoint == "" {
				in.Endpoint = v
			}
		case "last_handshake_time_sec":
			sec, _ = strconv.ParseInt(v, 10, 64)
		case "last_handshake_time_nsec":
			nsec, _ = strconv.ParseInt(v, 10, 64)
		case "rx_bytes":
			n, _ := strconv.ParseUint(v, 10, 64)
			in.RxBytes += n
		case "tx_bytes":
			n, _ := strconv.ParseUint(v, 10, 64)
			in.TxBytes += n
		}
	}
	flush()
}

// Resolve fills in each peer's EndpointAddr, preferring IPv4. Start does
// this itself; calling it first lets a new server's name be looked up while
// the old tunnel (and its DNS) is still up.
func Resolve(cfg *config.Config) error {
	return resolveEndpoints(cfg)
}

func resolveEndpoints(cfg *config.Config) error {
	for i := range cfg.Peers {
		p := &cfg.Peers[i]
		if p.Endpoint == "" || p.EndpointAddr.IsValid() {
			continue
		}
		host, port, err := p.EndpointHostPort()
		if err != nil {
			return err
		}
		addr, err := resolveHost(host)
		if err != nil {
			return fmt.Errorf("resolving server address %s: %w", host, err)
		}
		p.EndpointAddr = netip.AddrPortFrom(addr, port)
	}
	return nil
}

func resolveHost(host string) (netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return a.Unmap(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, a := range addrs {
		if a.Unmap().Is4() {
			return a.Unmap(), nil
		}
	}
	if len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("no addresses for %s", host)
	}
	return addrs[0], nil
}

// tunnelRoutes lists the routes that send the peers' AllowedIPs into the
// tunnel. A default route is split into two halves, as awg-quick does, so it
// wins over the system default route without replacing it.
func tunnelRoutes(cfg *config.Config) []netip.Prefix {
	var routes []netip.Prefix
	seen := map[netip.Prefix]bool{}
	add := func(p netip.Prefix) {
		if !seen[p] {
			seen[p] = true
			routes = append(routes, p)
		}
	}
	for _, peer := range cfg.Peers {
		for _, p := range peer.AllowedIPs {
			p = p.Masked()
			if p.Bits() != 0 {
				add(p)
			} else if p.Addr().Is4() {
				add(netip.MustParsePrefix("0.0.0.0/1"))
				add(netip.MustParsePrefix("128.0.0.0/1"))
			} else {
				add(netip.MustParsePrefix("::/1"))
				add(netip.MustParsePrefix("8000::/1"))
			}
		}
	}
	return routes
}

// endpointsInTunnel lists the peer endpoints that fall inside the tunnel's
// AllowedIPs. They need a route around the tunnel, or the encrypted packets
// would be sent into the tunnel itself.
func endpointsInTunnel(cfg *config.Config) []netip.Addr {
	var eps []netip.Addr
	seen := map[netip.Addr]bool{}
	for _, peer := range cfg.Peers {
		ep := peer.EndpointAddr.Addr()
		if !ep.IsValid() || seen[ep] {
			continue
		}
		for _, other := range cfg.Peers {
			for _, p := range other.AllowedIPs {
				if p.Masked().Contains(ep) && !seen[ep] {
					seen[ep] = true
					eps = append(eps, ep)
				}
			}
		}
	}
	return eps
}
