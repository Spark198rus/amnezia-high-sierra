package tunnel

import (
	"strconv"
	"strings"
)

// This file parses the output of the macOS tools the tunnel drives (netstat,
// route, scutil). It has no build tag so it can be tested anywhere.

// defaultGateway returns the gateway of the first "default" route in
// `netstat -nr -f inet` (or inet6) output, skipping interface ("link#")
// routes. It is the same rule awg-quick uses on macOS.
func defaultGateway(netstatOutput string) string {
	for _, line := range strings.Split(netstatOutput, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "default" && !strings.HasPrefix(f[1], "link#") {
			return f[1]
		}
	}
	return ""
}

// routeGetField returns a field such as "interface" from `route -n get` output.
func routeGetField(out, field string) string {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && k == field {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// dnsConfig is the part of a network service's DNS settings that is replaced
// while the tunnel is up and restored afterwards (the same fields the
// AmneziaVPN macOS daemon restores).
type dnsConfig struct {
	ServerAddresses []string `json:"serverAddresses,omitempty"`
	SearchDomains   []string `json:"searchDomains,omitempty"`
	DomainName      string   `json:"domainName,omitempty"`
	SortList        []string `json:"sortList,omitempty"`
}

// parseDNSDict parses scutil's "show" output for a DNS dictionary. ok is false
// when the key does not exist.
func parseDNSDict(out string) (cfg dnsConfig, ok bool) {
	if !strings.Contains(out, "<dictionary>") {
		return cfg, false
	}
	arrays := map[string][]string{}
	var current string // name of the array being read
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "}" {
			current = ""
			continue
		}
		k, v, found := strings.Cut(line, " : ")
		if !found {
			continue
		}
		if current != "" {
			if _, err := strconv.Atoi(k); err == nil {
				arrays[current] = append(arrays[current], v)
			}
			continue
		}
		if strings.HasPrefix(v, "<array>") {
			current = k
			arrays[k] = nil
			continue
		}
		if k == "DomainName" {
			cfg.DomainName = v
		}
	}
	cfg.ServerAddresses = arrays["ServerAddresses"]
	cfg.SearchDomains = arrays["SearchDomains"]
	cfg.SortList = arrays["SortList"]
	return cfg, true
}

// parseServiceIDs extracts network service IDs from scutil "list" output for
// keys under Setup:/Network/Service/.
func parseServiceIDs(out string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		_, key, found := strings.Cut(line, " = ")
		if !found {
			continue
		}
		parts := strings.Split(strings.TrimSpace(key), "/")
		// Setup: / Network / Service / <id> [/ ...]
		if len(parts) < 4 || parts[0] != "Setup:" || parts[1] != "Network" || parts[2] != "Service" {
			continue
		}
		if id := parts[3]; id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// scutilSetDNS returns scutil commands that store cfg at key.
func scutilSetDNS(key string, cfg dnsConfig) string {
	var b strings.Builder
	b.WriteString("d.init\n")
	addArray := func(name string, values []string) {
		if len(values) > 0 {
			b.WriteString("d.add " + name + " * " + strings.Join(values, " ") + "\n")
		}
	}
	addArray("ServerAddresses", cfg.ServerAddresses)
	addArray("SearchDomains", cfg.SearchDomains)
	addArray("SortList", cfg.SortList)
	if cfg.DomainName != "" {
		b.WriteString("d.add DomainName " + cfg.DomainName + "\n")
	}
	b.WriteString("set " + key + "\n")
	return b.String()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
