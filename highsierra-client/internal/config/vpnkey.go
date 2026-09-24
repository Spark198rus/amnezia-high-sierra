package config

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Load parses either the text of a .conf file or a "vpn://" key.
func Load(text string) (*Config, error) {
	if IsKey(text) {
		conf, err := DecodeKey(text)
		if err != nil {
			return nil, err
		}
		return Parse(conf)
	}
	return Parse(text)
}

// IsKey reports whether text looks like an AmneziaVPN "vpn://" key.
func IsKey(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "vpn://")
}

// amneziaServer is the part of an AmneziaVPN server config that a "vpn://"
// key carries and that matters here.
type amneziaServer struct {
	Containers       []map[string]json.RawMessage `json:"containers"`
	DefaultContainer string                       `json:"defaultContainer"`
	DNS1             string                       `json:"dns1"`
	DNS2             string                       `json:"dns2"`

	// Only subscription (Amnezia Premium / free) keys have these.
	APIKey    string          `json:"api_key"`
	APIConfig json.RawMessage `json:"api_config"`
	AuthData  json.RawMessage `json:"auth_data"`
}

// clientConfig is the JSON stored in a protocol's "last_config" string.
type clientConfig struct {
	Config string `json:"config"`
	MTU    string `json:"mtu"`
}

// DecodeKey turns an AmneziaVPN "vpn://" key for a self-hosted AmneziaWG (or
// WireGuard) server into .conf text.
//
// A key is base64url(qCompress(JSON)). The JSON lists the server's containers;
// the AmneziaWG one holds the ready-made client config in
// containers[].<protocol>.last_config.config.
func DecodeKey(key string) (string, error) {
	data := strings.TrimPrefix(strings.TrimSpace(key), "vpn://")
	data = strings.Join(strings.Fields(data), "")
	data = strings.NewReplacer("+", "-", "/", "_").Replace(strings.TrimRight(data, "="))
	raw, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("vpn:// key is not valid base64: %w", err)
	}
	if unpacked, err := qUncompress(raw); err == nil {
		raw = unpacked
	}

	var server amneziaServer
	if err := json.Unmarshal(raw, &server); err != nil {
		return "", fmt.Errorf("vpn:// key does not contain a server config: %w", err)
	}
	if server.APIKey != "" || len(server.APIConfig) > 0 || len(server.AuthData) > 0 {
		return "", errors.New("this is an Amnezia subscription key (Premium or free); " +
			"only keys for self-hosted servers are supported")
	}

	cc, err := server.findClientConfig()
	if err != nil {
		return "", err
	}

	conf := strings.NewReplacer("$PRIMARY_DNS", server.DNS1, "$SECONDARY_DNS", server.DNS2).Replace(cc.Config)
	if cc.MTU != "" && !hasMTU(conf) {
		conf = interfaceHeader.ReplaceAllString(conf, "${0}\nMTU = "+cc.MTU)
	}
	return conf, nil
}

var (
	interfaceHeader = regexp.MustCompile(`(?im)^[ \t]*\[Interface\][ \t\r]*$`)
	mtuLine         = regexp.MustCompile(`(?im)^[ \t]*MTU[ \t]*=`)
)

func hasMTU(conf string) bool { return mtuLine.MatchString(conf) }

// findClientConfig picks the default container's client config, or else the
// first one that holds a WireGuard-style config.
func (s *amneziaServer) findClientConfig() (clientConfig, error) {
	var first *clientConfig
	for _, container := range s.Containers {
		var name string
		_ = json.Unmarshal(container["container"], &name)
		fields := make([]string, 0, len(container))
		for field := range container {
			if field != "container" {
				fields = append(fields, field)
			}
		}
		sort.Strings(fields)
		for _, field := range fields {
			value := container[field]
			var proto struct {
				LastConfig string `json:"last_config"`
			}
			if json.Unmarshal(value, &proto) != nil || proto.LastConfig == "" {
				continue
			}
			var cc clientConfig
			if json.Unmarshal([]byte(proto.LastConfig), &cc) != nil ||
				!interfaceHeader.MatchString(cc.Config) {
				continue
			}
			if name != "" && name == s.DefaultContainer {
				return cc, nil
			}
			if first == nil {
				first = &cc
			}
		}
	}
	if first == nil {
		return clientConfig{}, errors.New("vpn:// key has no AmneziaWG or WireGuard connection in it " +
			"(export an AmneziaWG connection for this server from the AmneziaVPN app)")
	}
	return *first, nil
}

// qUncompress reverses Qt's qCompress: a 4-byte big-endian length followed by
// a zlib stream.
func qUncompress(b []byte) ([]byte, error) {
	if len(b) < 4 {
		return nil, errors.New("too short")
	}
	want := binary.BigEndian.Uint32(b[:4])
	r, err := zlib.NewReader(bytes.NewReader(b[4:]))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, 16<<20))
	if err != nil {
		return nil, err
	}
	if uint32(len(out)) != want {
		return nil, errors.New("length mismatch")
	}
	return out, nil
}
