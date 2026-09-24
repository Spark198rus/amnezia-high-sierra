package config

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

const (
	privKey = "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
	pubKey  = "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="
	pskKey  = "FpCyhws9cxwWoV4xELtfJvjJN+zQVRPISllRWgeopVE="
	hpKey   = "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw="
)

func hexOf(t *testing.T, b64 string) string {
	t.Helper()
	k, err := parseKey(b64)
	if err != nil {
		t.Fatal(err)
	}
	return k.hex()
}

func TestParseAmneziaWG(t *testing.T) {
	conf := `
# exported by AmneziaVPN
[Interface]
Address = 10.8.1.2/32, fd00::2
DNS = 1.1.1.1, 1.0.0.1 , corp.example
PrivateKey = ` + privKey + `
Jc = 4
Jmin = 10
Jmax = 50
S1 = 0
S2 = 0
S3 =
H1 = 100000 - 100100
H2 = 2
I1 = <b 0xc6000000><r 16> <t>
HeaderProtectionKey = ` + hpKey + `
RandomTrailers = on
DisableCookies = off
PostUp = rm -rf /

[Peer]
PublicKey = ` + pubKey + `
PresharedKey = ` + pskKey + `
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.com:55424
PersistentKeepalive = 25
`
	c, err := Parse(conf)
	if err != nil {
		t.Fatal(err)
	}

	in := c.Interface
	if got := len(in.Addresses); got != 2 || in.Addresses[1] != netip.MustParsePrefix("fd00::2/128") {
		t.Errorf("Addresses = %v", in.Addresses)
	}
	if len(in.DNS) != 2 || len(in.DNSSearch) != 1 || in.DNSSearch[0] != "corp.example" {
		t.Errorf("DNS = %v, search = %v", in.DNS, in.DNSSearch)
	}
	if in.MTUOrDefault() != DefaultMTU {
		t.Errorf("MTU = %d", in.MTUOrDefault())
	}
	if host, port, err := c.Peers[0].EndpointHostPort(); err != nil || host != "vpn.example.com" || port != 55424 {
		t.Errorf("endpoint = %q %d %v", host, port, err)
	}

	c.Peers[0].EndpointAddr = netip.MustParseAddrPort("203.0.113.7:55424")
	want := strings.Join([]string{
		"private_key=" + hexOf(t, privKey),
		"listen_port=0",
		"replace_peers=true",
		"jc=4",
		"jmin=10",
		"jmax=50",
		"s1=0",
		"s2=0",
		"h1=100000-100100",
		"h2=2",
		"i1=<b 0xc6000000><r 16> <t>",
		"header_protection_key=" + hexOf(t, hpKey),
		"random_trailers=1",
		"disable_cookies=0",
		"public_key=" + hexOf(t, pubKey),
		"preshared_key=" + hexOf(t, pskKey),
		"endpoint=203.0.113.7:55424",
		"persistent_keepalive_interval=25",
		"replace_allowed_ips=true",
		"allowed_ip=0.0.0.0/0",
		"allowed_ip=::/0",
	}, "\n") + "\n"
	if got := c.UAPI(); got != want {
		t.Errorf("UAPI mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseCaseAndCRLF(t *testing.T) {
	conf := "[INTERFACE]\r\nprivatekey=" + privKey + "\r\naddress=10.0.0.2\r\nmtu = 1280\r\n" +
		"[peer]\r\npublickey=" + pubKey + "\r\nallowedips=10.0.0.0/24\r\n"
	c, err := Parse(conf)
	if err != nil {
		t.Fatal(err)
	}
	if c.Interface.MTU != 1280 || c.Interface.Addresses[0] != netip.MustParsePrefix("10.0.0.2/32") {
		t.Errorf("got %+v", c.Interface)
	}
	if c.Peers[0].EndpointAddr.IsValid() || strings.Contains(c.UAPI(), "endpoint=") {
		t.Error("peer without Endpoint must not get one")
	}
}

func TestParseErrors(t *testing.T) {
	peer := "\n[Peer]\nPublicKey = " + pubKey + "\nAllowedIPs = 0.0.0.0/0\n"
	for name, conf := range map[string]string{
		"no private key": "[Interface]\nAddress = 10.0.0.2/32\n" + peer,
		"no address":     "[Interface]\nPrivateKey = " + privKey + "\n" + peer,
		"no peer":        "[Interface]\nPrivateKey = " + privKey + "\nAddress = 10.0.0.2/32\n",
		"unknown key":    "[Interface]\nPrivateKey = " + privKey + "\nAddress = 10.0.0.2/32\nBogus = 1\n" + peer,
		"bad key":        "[Interface]\nPrivateKey = abc\nAddress = 10.0.0.2/32\n" + peer,
		"bad bool":       "[Interface]\nPrivateKey = " + privKey + "\nAddress = 10.0.0.2/32\nRandomTrailers = maybe\n" + peer,
		"bad endpoint":   "[Interface]\nPrivateKey = " + privKey + "\nAddress = 10.0.0.2/32\n" + peer + "Endpoint = nope\n",
		"no section":     "PrivateKey = " + privKey + "\n",
	} {
		if _, err := Parse(conf); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// makeKey builds a key the way AmneziaVPN's ExportController does.
func makeKey(t *testing.T, server map[string]interface{}) string {
	t.Helper()
	js, err := json.Marshal(server)
	if err != nil {
		t.Fatal(err)
	}
	var z bytes.Buffer
	binary.Write(&z, binary.BigEndian, uint32(len(js)))
	w, _ := zlib.NewWriterLevel(&z, 8)
	w.Write(js)
	w.Close()
	return "vpn://" + base64.RawURLEncoding.EncodeToString(z.Bytes())
}

func TestDecodeKey(t *testing.T) {
	native := "[Interface]\nAddress = 10.8.1.5/32\nDNS = $PRIMARY_DNS, $SECONDARY_DNS\nPrivateKey = " + privKey +
		"\nJc = 3\n\n[Peer]\nPublicKey = " + pubKey + "\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = 198.51.100.4:55424\n"
	lastConfig, _ := json.Marshal(map[string]string{"config": native, "mtu": "1280"})
	key := makeKey(t, map[string]interface{}{
		"hostName":         "198.51.100.4",
		"dns1":             "1.1.1.1",
		"dns2":             "1.0.0.1",
		"defaultContainer": "amnezia-awg2",
		"containers": []interface{}{
			map[string]interface{}{"container": "amnezia-openvpn", "openvpn": map[string]string{"last_config": `{"config":"client"}`}},
			map[string]interface{}{"container": "amnezia-awg2", "awg": map[string]string{"last_config": string(lastConfig)}},
		},
	})

	conf, err := DecodeKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "DNS = 1.1.1.1, 1.0.0.1") || !strings.Contains(conf, "MTU = 1280") {
		t.Errorf("decoded config:\n%s", conf)
	}

	c, err := Load(key)
	if err != nil {
		t.Fatal(err)
	}
	if c.Interface.MTU != 1280 || len(c.Interface.DNS) != 2 || c.Peers[0].Endpoint != "198.51.100.4:55424" {
		t.Errorf("got %+v", c)
	}
}

func TestDecodeKeyRejectsSubscriptions(t *testing.T) {
	key := makeKey(t, map[string]interface{}{"api_key": "x", "containers": []interface{}{}})
	if _, err := DecodeKey(key); err == nil || !strings.Contains(err.Error(), "subscription") {
		t.Errorf("err = %v", err)
	}
}

func TestDecodeKeyWithoutAWG(t *testing.T) {
	key := makeKey(t, map[string]interface{}{"containers": []interface{}{
		map[string]interface{}{"container": "amnezia-openvpn", "openvpn": map[string]string{"last_config": `{"config":"client"}`}},
	}})
	if _, err := DecodeKey(key); err == nil {
		t.Error("expected an error")
	}
}
