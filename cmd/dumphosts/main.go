// dumphosts is a tiny dev helper: prints what ListHosts() would return.
// Run with:  SSHFWD_SSH_CONFIG=/path/to/config go run ./cmd/dumphosts
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kevinburke/ssh_config"
)

type host struct {
	Alias    string
	HostName string
	User     string
	Port     string
}

func main() {
	path := os.Getenv("SSHFWD_SSH_CONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".ssh", "config")
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		os.Exit(1)
	}

	seen := map[string]bool{}
	var hosts []host
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
			hn, _ := cfg.Get(alias, "HostName")
			if hn == "" {
				hn = alias
			}
			user, _ := cfg.Get(alias, "User")
			port, _ := cfg.Get(alias, "Port")
			hosts = append(hosts, host{alias, hn, user, port})
		}
	}
	sort.Slice(hosts, func(i, j int) bool {
		return strings.ToLower(hosts[i].Alias) < strings.ToLower(hosts[j].Alias)
	})
	fmt.Printf("%-15s %-30s %-15s %s\n", "ALIAS", "HOSTNAME", "USER", "PORT")
	for _, h := range hosts {
		fmt.Printf("%-15s %-30s %-15s %s\n", h.Alias, h.HostName, h.User, h.Port)
	}
	fmt.Printf("\n%d host(s) found\n", len(hosts))
}
