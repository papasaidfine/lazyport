package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// Host is a single SSH host entry from ~/.ssh/config.
type Host struct {
	Alias    string // the name a user types (Host alias)
	HostName string // resolved HostName, falls back to Alias if not set
	User     string // resolved User (may be empty)
	Port     string // resolved Port (may be empty)
}

// ListHosts parses the user's ~/.ssh/config and returns all non-wildcard
// host entries, sorted alphabetically. A missing config file yields an empty
// slice rather than an error so first-time users still see a usable UI.
func ListHosts() ([]Host, error) {
	path, err := defaultSSHConfigPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open ssh config: %w", err)
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode ssh config: %w", err)
	}

	seen := map[string]bool{}
	var hosts []Host
	for _, h := range cfg.Hosts {
		for _, pat := range h.Patterns {
			alias := pat.String()
			if alias == "" || strings.ContainsAny(alias, "*?!") {
				continue
			}
			if seen[alias] {
				continue
			}
			seen[alias] = true

			hostName, _ := cfg.Get(alias, "HostName")
			if hostName == "" {
				hostName = alias
			}
			user, _ := cfg.Get(alias, "User")
			port, _ := cfg.Get(alias, "Port")

			hosts = append(hosts, Host{
				Alias:    alias,
				HostName: hostName,
				User:     user,
				Port:     port,
			})
		}
	}

	sort.Slice(hosts, func(i, j int) bool {
		return strings.ToLower(hosts[i].Alias) < strings.ToLower(hosts[j].Alias)
	})
	return hosts, nil
}

func defaultSSHConfigPath() (string, error) {
	if p := os.Getenv("LAZYPORT_SSH_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}
