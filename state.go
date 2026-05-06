package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// State is the on-disk record of which tunnels were active per host the last
// time lazyport was running. We use it to offer reconnection on startup.
type State struct {
	Hosts map[string][]Tunnel `json:"hosts"`
}

// configDir returns the platform-appropriate lazyport config directory.
// It does NOT create the directory.
func configDir() (string, error) {
	if p := os.Getenv("LAZYPORT_CONFIG_DIR"); p != "" {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "lazyport"), nil
	}
	// XDG-friendly default on unix-likes.
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "lazyport"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazyport"), nil
}

func statePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// LoadState reads the persisted state. A missing file yields an empty State.
func LoadState() (*State, error) {
	p, err := statePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &State{Hosts: map[string][]Tunnel{}}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	s := &State{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Hosts == nil {
		s.Hosts = map[string][]Tunnel{}
	}
	return s, nil
}

// Save writes the state atomically (write-temp + rename) to avoid a torn file
// if the process is killed mid-write.
func (s *State) Save() error {
	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "state-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Get returns the persisted tunnel list for a host (sorted by port).
func (s *State) Get(alias string) []Tunnel {
	if s == nil || s.Hosts == nil {
		return nil
	}
	out := append([]Tunnel(nil), s.Hosts[alias]...)
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// Set replaces the persisted tunnel list for a host.
func (s *State) Set(alias string, tunnels []Tunnel) {
	if s.Hosts == nil {
		s.Hosts = map[string][]Tunnel{}
	}
	if len(tunnels) == 0 {
		delete(s.Hosts, alias)
		return
	}
	cp := append([]Tunnel(nil), tunnels...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Port < cp[j].Port })
	s.Hosts[alias] = cp
}
