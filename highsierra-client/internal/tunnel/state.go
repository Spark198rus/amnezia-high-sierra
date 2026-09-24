package tunnel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// recoveryState records the system changes that outlive the tunnel process,
// so a restarted daemon can undo them after a crash. Routes through the utun
// interface need no record: macOS removes them when the interface goes away.
// The file lives under /var/run, which is cleared at boot, as are the changes.
type recoveryState struct {
	Interface      string                `json:"interface"`
	EndpointRoutes []string              `json:"endpointRoutes,omitempty"`
	DNSBackups     map[string]*dnsConfig `json:"dnsBackups,omitempty"` // nil: the service had no DNS key
}

func loadState(path string) (*recoveryState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s recoveryState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *recoveryState) save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
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

func removeState(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
