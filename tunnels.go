package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Tunnel describes a single local→remote port forward attached to a host.
type Tunnel struct {
	Port int `json:"port"`
}

// controlSocketPath returns the per-host ControlMaster socket path.
//
// On Windows the unix-socket ControlMaster pattern is unsupported, but the
// path is still where we would track liveness if a future ssh build adds it.
func controlSocketPath(alias string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return filepath.Join(dir, "ctrl-"+sanitizeAlias(alias)), nil
}

// sanitizeAlias makes an SSH alias safe to use as a filename component.
func sanitizeAlias(alias string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(alias)
}

// IsConnected reports whether a usable ControlMaster socket exists for alias.
// It does this by asking ssh itself with `-O check`, which is the only way to
// know whether the master is alive (a socket file alone isn't proof).
func IsConnected(alias string) bool {
	sock, err := controlSocketPath(alias)
	if err != nil {
		return false
	}
	if _, err := os.Stat(sock); err != nil {
		return false
	}
	cmd := exec.Command("ssh", "-S", sock, "-O", "check", alias)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// Connect establishes a ControlMaster session for alias. It returns nil if
// the master is already running.
func Connect(alias string) error {
	if IsConnected(alias) {
		return nil
	}
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	// -M master, -S socket path, -f background, -N no remote command.
	// ServerAliveInterval keeps the master from going idle behind a NAT.
	cmd := exec.Command("ssh",
		"-M",
		"-S", sock,
		"-fN",
		"-o", "ControlPersist=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ExitOnForwardFailure=yes",
		alias,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh master for %s: %w: %s", alias, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Disconnect tears down the ControlMaster (and all tunnels riding it).
func Disconnect(alias string) error {
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	if _, err := os.Stat(sock); err != nil {
		return nil // nothing to do
	}
	cmd := exec.Command("ssh", "-S", sock, "-O", "exit", alias)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// stale socket: try to clean up so the next connect works.
		_ = os.Remove(sock)
		return fmt.Errorf("ssh exit for %s: %w: %s", alias, err, strings.TrimSpace(string(out)))
	}
	_ = os.Remove(sock)
	return nil
}

// Forward adds a local→remote port forward to an existing master.
// localPort == remotePort, both bound to localhost on the remote side.
func Forward(alias string, port int) error {
	if !IsConnected(alias) {
		if err := Connect(alias); err != nil {
			return err
		}
	}
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	spec := fmt.Sprintf("%d:localhost:%d", port, port)
	cmd := exec.Command("ssh", "-S", sock, "-O", "forward", "-L", spec, alias)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh forward %s on %s: %w: %s", spec, alias, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Cancel removes a previously-added forward.
func Cancel(alias string, port int) error {
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	if _, err := os.Stat(sock); err != nil {
		return errors.New("no active master for " + alias)
	}
	spec := fmt.Sprintf("%d:localhost:%d", port, port)
	cmd := exec.Command("ssh", "-S", sock, "-O", "cancel", "-L", spec, alias)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh cancel %s on %s: %w: %s", spec, alias, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SupportsControlMaster reports whether the running OS can use the
// ControlMaster pattern. Windows OpenSSH historically lacks unix-socket
// multiplexing; we surface that to the UI rather than failing silently.
func SupportsControlMaster() bool {
	return runtime.GOOS != "windows"
}
