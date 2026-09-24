// Package config parses AmneziaWG tunnel configurations: the wg-quick style
// .conf format and the "vpn://" keys that the AmneziaVPN app shares.
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// DefaultMTU is used when a config does not set one. It matches the AmneziaVPN
// desktop default for AmneziaWG, which leaves room for the obfuscation padding.
const DefaultMTU = 1376

// Config is a parsed tunnel configuration.
type Config struct {
	Interface Interface
	Peers     []Peer
}

// Interface holds the [Interface] section.
type Interface struct {
	PrivateKey Key
	ListenPort uint16 // 0 picks a random port
	Addresses  []netip.Prefix
	DNS        []netip.Addr
	DNSSearch  []string // non-address DNS entries are search domains, as in wg-quick
	MTU        int      // 0 means DefaultMTU
	// AWG holds the AmneziaWG obfuscation settings as UAPI key/value pairs,
	// in the order they appear in the config.
	AWG []Param
}

// Param is one UAPI key/value pair.
type Param struct {
	Key   string
	Value string
}

// Peer holds one [Peer] section.
type Peer struct {
	PublicKey           Key
	PresharedKey        Key // zero if unset
	Endpoint            string
	AllowedIPs          []netip.Prefix
	PersistentKeepalive string // UAPI value, empty if unset

	// EndpointAddr is the resolved Endpoint. The caller fills it in before
	// calling UAPI, because resolving needs the network.
	EndpointAddr netip.AddrPort
}

// Key is a Curve25519 key or a preshared key.
type Key [32]byte

// IsZero reports whether the key is unset.
func (k Key) IsZero() bool { return k == Key{} }

func (k Key) hex() string { return hex.EncodeToString(k[:]) }

// Base64 returns the key in the usual WireGuard text form.
func (k Key) Base64() string { return base64.StdEncoding.EncodeToString(k[:]) }

func parseKey(s string) (Key, error) {
	var k Key
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != len(k) {
		return k, errors.New("key must be 32 bytes of base64")
	}
	copy(k[:], b)
	return k, nil
}

// awgKeys maps AmneziaWG [Interface] keys (lower-cased) to their UAPI names.
// Their values are passed through unchanged; amneziawg-go validates them.
var awgKeys = map[string]string{
	"jc":                     "jc",
	"jmin":                   "jmin",
	"jmax":                   "jmax",
	"s1":                     "s1",
	"s2":                     "s2",
	"s3":                     "s3",
	"s4":                     "s4",
	"h1":                     "h1",
	"h2":                     "h2",
	"h3":                     "h3",
	"h4":                     "h4",
	"i1":                     "i1",
	"i2":                     "i2",
	"i3":                     "i3",
	"i4":                     "i4",
	"i5":                     "i5",
	"contentpaddingaddition": "content_padding_addition",
	"rekeyaftertime":         "rekey_after_time",
	"rekeytimeout":           "rekey_timeout",
	"rejectaftertime":        "reject_after_time",
	"keepalivetimeout":       "keepalive_timeout",
	"maxhandshakeattempts":   "max_handshake_attempts",
}

// awgBoolKeys are AmneziaWG on/off settings, which UAPI takes as 1/0.
var awgBoolKeys = map[string]string{
	"randomtrailers": "random_trailers",
	"disablecookies": "disable_cookies",
}

// ignoredKeys are wg-quick keys that have no meaning here. Hook scripts are
// deliberately never run: they would execute as root.
var ignoredKeys = map[string]bool{
	"table":      true,
	"saveconfig": true,
	"preup":      true,
	"postup":     true,
	"predown":    true,
	"postdown":   true,
	"fwmark":     true, // Linux only
}

// Parse parses a wg-quick style AmneziaWG config. It follows the awg tool's
// rules: '#' starts a comment, keys are case-insensitive, and whitespace inside
// values is ignored except in the I1-I5 values.
func Parse(text string) (*Config, error) {
	var c Config
	var peer *Peer
	section := ""
	seenInterface := false

	for n, line := range strings.Split(text, "\n") {
		lineNo := n + 1
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") {
			switch strings.ToLower(line) {
			case "[interface]":
				if seenInterface {
					return nil, fmt.Errorf("line %d: more than one [Interface] section", lineNo)
				}
				seenInterface = true
				section = "interface"
			case "[peer]":
				section = "peer"
				c.Peers = append(c.Peers, Peer{})
				peer = &c.Peers[len(c.Peers)-1]
			default:
				return nil, fmt.Errorf("line %d: unknown section %s", lineNo, line)
			}
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected \"Key = Value\"", lineNo)
		}
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		value = strings.TrimSpace(value)
		if !isSpecialJunkKey(key) {
			value = strings.Join(strings.Fields(value), "")
		}
		// AmneziaVPN templates leave unused parameters empty.
		if value == "" {
			continue
		}

		var err error
		switch section {
		case "interface":
			err = c.Interface.set(key, value)
		case "peer":
			err = peer.set(key, value)
		default:
			err = errors.New("setting is outside of a section")
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", lineNo, name, err)
		}
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func isSpecialJunkKey(key string) bool {
	return len(key) == 2 && key[0] == 'i' && key[1] >= '1' && key[1] <= '5'
}

func (in *Interface) set(key, value string) error {
	if uapiKey, ok := awgKeys[key]; ok {
		in.AWG = append(in.AWG, Param{uapiKey, value})
		return nil
	}
	if ignoredKeys[key] {
		return nil
	}

	var err error
	switch key {
	case "privatekey":
		in.PrivateKey, err = parseKey(value)
	case "listenport":
		var port uint64
		port, err = strconv.ParseUint(value, 10, 16)
		in.ListenPort = uint16(port)
	case "address":
		for _, s := range strings.Split(value, ",") {
			p, perr := parsePrefix(s)
			if perr != nil {
				return perr
			}
			in.Addresses = append(in.Addresses, p)
		}
	case "dns":
		for _, s := range strings.Split(value, ",") {
			if a, aerr := netip.ParseAddr(s); aerr == nil {
				in.DNS = append(in.DNS, a)
			} else if s != "" {
				in.DNSSearch = append(in.DNSSearch, s)
			}
		}
	case "mtu":
		in.MTU, err = strconv.Atoi(value)
		if err == nil && (in.MTU < 576 || in.MTU > 9000) {
			err = errors.New("must be between 576 and 9000")
		}
	case "headerprotectionkey":
		var k Key
		if k, err = parseKey(value); err == nil {
			in.AWG = append(in.AWG, Param{"header_protection_key", k.hex()})
		}
	case "randomtrailers", "disablecookies":
		var b string
		if b, err = parseBool(value); err == nil {
			in.AWG = append(in.AWG, Param{awgBoolKeys[key], b})
		}
	default:
		return errors.New("not a supported [Interface] setting")
	}
	return err
}

func (p *Peer) set(key, value string) error {
	var err error
	switch key {
	case "publickey":
		p.PublicKey, err = parseKey(value)
	case "presharedkey":
		p.PresharedKey, err = parseKey(value)
	case "endpoint":
		_, _, err = splitEndpoint(value)
		p.Endpoint = value
	case "allowedips":
		for _, s := range strings.Split(value, ",") {
			pfx, perr := parsePrefix(s)
			if perr != nil {
				return perr
			}
			p.AllowedIPs = append(p.AllowedIPs, pfx)
		}
	case "persistentkeepalive":
		if strings.EqualFold(value, "off") {
			value = "0"
		}
		p.PersistentKeepalive = value
	case "advancedsecurity":
		// Kernel-module only option; the userspace implementation has no use for it.
	default:
		return errors.New("not a supported [Peer] setting")
	}
	return err
}

func (c *Config) validate() error {
	if c.Interface.PrivateKey.IsZero() {
		return errors.New("[Interface] has no PrivateKey")
	}
	if len(c.Interface.Addresses) == 0 {
		return errors.New("[Interface] has no Address")
	}
	if len(c.Peers) == 0 {
		return errors.New("config has no [Peer] section")
	}
	for i, p := range c.Peers {
		if p.PublicKey.IsZero() {
			return fmt.Errorf("[Peer] %d has no PublicKey", i+1)
		}
	}
	return nil
}

// MTUOrDefault returns the configured MTU or DefaultMTU.
func (in *Interface) MTUOrDefault() int {
	if in.MTU == 0 {
		return DefaultMTU
	}
	return in.MTU
}

// parsePrefix accepts "10.0.0.1/24" as well as a bare address, which means a
// single host.
func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func parseBool(s string) (string, error) {
	switch strings.ToLower(s) {
	case "on", "true", "yes", "1":
		return "1", nil
	case "off", "false", "no", "0":
		return "0", nil
	}
	return "", fmt.Errorf("%q is not on/off", s)
}

// splitEndpoint splits "host:port" or "[v6]:port".
func splitEndpoint(s string) (host string, port uint16, err error) {
	h, p, err := net.SplitHostPort(s)
	if err != nil {
		return "", 0, err
	}
	n, err := strconv.ParseUint(p, 10, 16)
	if err != nil || n == 0 {
		return "", 0, fmt.Errorf("bad port in %q", s)
	}
	return h, uint16(n), nil
}

// EndpointHostPort splits the peer's Endpoint into host and port.
func (p *Peer) EndpointHostPort() (string, uint16, error) {
	return splitEndpoint(p.Endpoint)
}

// UAPI renders the config as an amneziawg-go "set" operation. Peers without
// a resolved EndpointAddr are configured without an endpoint.
func (c *Config) UAPI() string {
	var b strings.Builder
	w := func(k, v string) { b.WriteString(k + "=" + v + "\n") }

	w("private_key", c.Interface.PrivateKey.hex())
	w("listen_port", strconv.Itoa(int(c.Interface.ListenPort)))
	w("replace_peers", "true")
	for _, p := range c.Interface.AWG {
		w(p.Key, p.Value)
	}
	for _, p := range c.Peers {
		w("public_key", p.PublicKey.hex())
		if !p.PresharedKey.IsZero() {
			w("preshared_key", p.PresharedKey.hex())
		}
		if p.EndpointAddr.IsValid() {
			w("endpoint", p.EndpointAddr.String())
		}
		if p.PersistentKeepalive != "" {
			w("persistent_keepalive_interval", p.PersistentKeepalive)
		}
		w("replace_allowed_ips", "true")
		for _, a := range p.AllowedIPs {
			w("allowed_ip", a.String())
		}
	}
	return b.String()
}
