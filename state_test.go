package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SSHFWD_CONFIG_DIR", dir)

	s, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState empty: %v", err)
	}
	if len(s.Hosts) != 0 {
		t.Fatalf("expected empty initial state, got %v", s.Hosts)
	}

	s.Set("dev-server", []Tunnel{{Port: 8080}, {Port: 5432}})
	s.Set("staging", []Tunnel{{Port: 9090}})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	s2, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState round-trip: %v", err)
	}

	wantDev := []Tunnel{{Port: 5432}, {Port: 8080}}
	if got := s2.Get("dev-server"); !reflect.DeepEqual(got, wantDev) {
		t.Errorf("dev-server tunnels: got %v want %v", got, wantDev)
	}
	if got := s2.Get("staging"); !reflect.DeepEqual(got, []Tunnel{{Port: 9090}}) {
		t.Errorf("staging tunnels: got %v", got)
	}
}

func TestStateSetEmptyDeletes(t *testing.T) {
	s := &State{Hosts: map[string][]Tunnel{}}
	s.Set("foo", []Tunnel{{Port: 1234}})
	s.Set("foo", nil)
	if _, ok := s.Hosts["foo"]; ok {
		t.Errorf("expected empty Set to remove host entry")
	}
}
