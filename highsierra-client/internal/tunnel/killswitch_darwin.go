//go:build darwin

package tunnel

import (
	"errors"
	"log"
	"os"
	"sync"
)

const pfctlCmd = "/sbin/pfctl"

// KillSwitch blocks traffic that would leave the Mac outside the tunnel. It
// stays on across tunnels, so switching servers never opens a gap, and
// across a daemon crash: only Disable lifts it.
type KillSwitch struct {
	statePath string

	mu    sync.Mutex
	state *killSwitchState // nil while off
}

// LoadKillSwitch returns the kill switch, still on if a previous daemon
// process left it on.
func LoadKillSwitch(statePath string) *KillSwitch {
	k := &KillSwitch{statePath: statePath}
	if s, err := loadKillSwitchState(statePath); err == nil {
		log.Printf("the kill switch is still on from before; traffic outside the tunnel stays blocked")
		k.state = s
		k.ensure()
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("reading %s: %v", statePath, err)
	}
	return k
}

// Active reports whether the kill switch is blocking traffic.
func (k *KillSwitch) Active() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.state != nil
}

// apply replaces the rules with ones for spec and makes sure pf enforces
// them. pf swaps an anchor's rules in one step, so nothing leaks meanwhile.
func (k *KillSwitch) apply(spec killSwitchSpec) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.applyLocked(spec)
}

func (k *KillSwitch) applyLocked(spec killSwitchSpec) (err error) {
	if err := pfEnsureAnchorRef(); err != nil {
		return err
	}
	if _, err := runInput(killSwitchRules(spec), pfctlCmd, "-a", pfAnchor, "-f", "-"); err != nil {
		return err
	}
	if k.state == nil {
		// Don't leave rules in force that nothing records.
		defer func() {
			if err != nil {
				flushAnchor()
			}
		}()
	}
	token := ""
	if k.state != nil {
		token = k.state.Token
	}
	if token == "" || !pfEnabled() {
		// "pfctl -E" enables pf and takes a reference on it, which Disable
		// gives back; pf stays on while anything else still holds one.
		out, err := run(pfctlCmd, "-E")
		if err != nil {
			return err
		}
		if token = parsePfToken(out); token == "" {
			return errors.New("pfctl -E did not return a token")
		}
	}
	k.state = &killSwitchState{Token: token, Spec: spec}
	if err := k.state.save(k.statePath); err != nil {
		log.Printf("saving %s: %v", k.statePath, err)
	}
	return nil
}

// Disable removes the rules and gives back the pf reference.
func (k *KillSwitch) Disable() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.state == nil {
		return
	}
	flushAnchor()
	if k.state.Token != "" {
		run(pfctlCmd, "-X", k.state.Token)
	}
	k.state = nil
	if err := os.Remove(k.statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("removing %s: %v", k.statePath, err)
	}
}

// ensure puts the rules back if something reloaded or turned off pf, which
// macOS does when, for example, Internet Sharing is switched on.
func (k *KillSwitch) ensure() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.state == nil {
		return
	}
	if pfEnabled() && pfAnchorReferenced() && pfAnchorLoaded() {
		return
	}
	log.Printf("the kill switch rules were removed from pf; putting them back")
	if err := k.applyLocked(k.state.Spec); err != nil {
		log.Printf("restoring the kill switch: %v", err)
	}
}

// flushAnchor removes the kill switch rules and tables. Anchor-scoped flushes
// leave the rest of pf, including its connection states, alone.
func flushAnchor() {
	for _, what := range []string{"rules", "Tables"} {
		if _, err := run(pfctlCmd, "-a", pfAnchor, "-F", what); err != nil {
			log.Printf("removing the kill switch %s: %v", what, err)
		}
	}
}

func pfEnabled() bool {
	out, err := runStdout("", pfctlCmd, "-s", "info")
	return err == nil && containsLine(out, "Status: Enabled")
}

func pfAnchorReferenced() bool {
	out, err := runStdout("", pfctlCmd, "-sr")
	return err == nil && hasAnchorRef(out)
}

func pfAnchorLoaded() bool {
	out, err := runStdout("", pfctlCmd, "-a", pfAnchor, "-sr")
	return err == nil && len(out) > 0
}

// pfEnsureAnchorRef adds the awg-hs anchor to the end of the main filter
// rules, keeping the rest (macOS's own anchors) as they are. This is how the
// AmneziaVPN macOS daemon hooks in its firewall anchors too.
func pfEnsureAnchorRef() error {
	rules, err := runStdout("", pfctlCmd, "-sr")
	if err != nil {
		return err
	}
	if hasAnchorRef(rules) {
		return nil
	}
	// -R replaces only the filter rules; scrub and NAT rules stay loaded.
	_, err = runInput(withAnchorRef(rules), pfctlCmd, "-R", "-f", "-")
	return err
}
