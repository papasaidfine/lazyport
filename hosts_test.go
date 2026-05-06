package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListHostsSkipsWildcards(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfg := `
Host bastion
    HostName bastion.example.com
    User alice

Host dev-server
    HostName 10.0.0.1

Host prod ?prefix
    User bob

Host !excluded
    User mallory

Host *
    User defaultuser
`
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAZYPORT_SSH_CONFIG", path)

	hosts, err := ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}

	got := map[string]Host{}
	for _, h := range hosts {
		got[h.Alias] = h
	}

	for _, want := range []string{"bastion", "dev-server", "prod"} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected host %q in result, got %+v", want, got)
		}
	}
	for _, unwanted := range []string{"*", "?prefix", "!excluded"} {
		if _, ok := got[unwanted]; ok {
			t.Errorf("did not expect host %q in result", unwanted)
		}
	}
	if h := got["bastion"]; h.HostName != "bastion.example.com" || h.User != "alice" {
		t.Errorf("bastion not resolved correctly: %+v", h)
	}
	// dev-server has no explicit User — make sure that's tolerated.
	if h := got["dev-server"]; h.HostName != "10.0.0.1" {
		t.Errorf("dev-server hostname wrong: %+v", h)
	}
}

func TestListHostsMissingFile(t *testing.T) {
	t.Setenv("LAZYPORT_SSH_CONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	hosts, err := ListHosts()
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected empty hosts, got %v", hosts)
	}
}
