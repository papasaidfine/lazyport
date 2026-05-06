package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Tunnel describes a single local→remote port forward attached to a host.
type Tunnel struct {
	Port int `json:"port"`
}

// backend abstracts how port forwards are realised on a given platform.
//
// Two implementations exist:
//
//   - masterBackend: one long-lived `ssh -M -fN` process per host, with a
//     unix-socket ControlMaster; subsequent `-O forward` calls multiplex
//     forwards through that socket. Cheap, but unix-socket multiplexing
//     isn't reliably supported on Windows OpenSSH (failure mode:
//     "getsockname failed: Not a socket").
//
//   - procBackend: one `ssh -N -L ...` subprocess per forward, tracked by
//     PID. Slower (one auth per forward) but works everywhere ssh works.
type backend interface {
	IsConnected(alias string) bool
	Connect(alias string) error
	Disconnect(alias string) error
	Forward(alias string, port int) error
	Cancel(alias string, port int) error
}

var activeBackend = pickBackend()

// pickBackend chooses the implementation for the current platform.
//
// Windows always gets procBackend because Windows OpenSSH's ControlMaster
// implementation is incomplete. LAZYPORT_NO_CONTROLMASTER=1 forces
// procBackend on any platform — useful when ControlMaster fails on a
// non-Windows host (NFS homedirs, restrictive sandboxes, …).
func pickBackend() backend {
	if runtime.GOOS == "windows" || os.Getenv("LAZYPORT_NO_CONTROLMASTER") != "" {
		return newProcBackend()
	}
	return &masterBackend{}
}

// SupportsControlMaster reports whether the active backend uses ControlMaster.
func SupportsControlMaster() bool {
	_, ok := activeBackend.(*masterBackend)
	return ok
}

// Top-level entry points kept stable so callers don't have to know about the
// interface.
func IsConnected(alias string) bool        { return activeBackend.IsConnected(alias) }
func Connect(alias string) error           { return activeBackend.Connect(alias) }
func Disconnect(alias string) error        { return activeBackend.Disconnect(alias) }
func Forward(alias string, port int) error { return activeBackend.Forward(alias, port) }
func Cancel(alias string, port int) error  { return activeBackend.Cancel(alias, port) }

// ---------------- masterBackend (ControlMaster) ----------------

type masterBackend struct{}

// controlSocketPath returns the per-host ControlMaster socket path. Used only
// by masterBackend, but lives at package scope because it touches configDir().
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

func sanitizeAlias(alias string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(alias)
}

func (m *masterBackend) IsConnected(alias string) bool {
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

func (m *masterBackend) Connect(alias string) error {
	if m.IsConnected(alias) {
		return nil
	}
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	cmd := exec.Command("ssh",
		"-M",
		"-S", sock,
		"-fN",
		"-o", "ControlPersist=yes",
		alias,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh master for %s: %w: %s", alias, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *masterBackend) Disconnect(alias string) error {
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	if _, err := os.Stat(sock); err != nil {
		return nil
	}
	cmd := exec.Command("ssh", "-S", sock, "-O", "exit", alias)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(sock)
		return fmt.Errorf("ssh exit for %s: %w: %s", alias, err, strings.TrimSpace(string(out)))
	}
	_ = os.Remove(sock)
	return nil
}

func (m *masterBackend) Forward(alias string, port int) error {
	if !m.IsConnected(alias) {
		if err := m.Connect(alias); err != nil {
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

func (m *masterBackend) Cancel(alias string, port int) error {
	sock, err := controlSocketPath(alias)
	if err != nil {
		return err
	}
	if _, err := os.Stat(sock); err != nil {
		return fmt.Errorf("no active master for %s", alias)
	}
	spec := fmt.Sprintf("%d:localhost:%d", port, port)
	cmd := exec.Command("ssh", "-S", sock, "-O", "cancel", "-L", spec, alias)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh cancel %s on %s: %w: %s", spec, alias, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------------- procBackend (one ssh per forward) ----------------

type procBackend struct {
	mu       sync.Mutex
	intended map[string]bool              // hosts the user has clicked "connect" on
	procs    map[string]map[int]*exec.Cmd // alias → port → running ssh subprocess
}

func newProcBackend() *procBackend {
	return &procBackend{
		intended: map[string]bool{},
		procs:    map[string]map[int]*exec.Cmd{},
	}
}

func (p *procBackend) IsConnected(alias string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.intended[alias] || len(p.procs[alias]) > 0
}

// Connect on the proc backend has nothing to start (no master), so we use it
// to verify the host is reachable with the user's existing key/agent setup.
// That way, hitting Enter on a host gives feedback right away rather than
// waiting until the user tries to forward a port.
func (p *procBackend) Connect(alias string) error {
	cmd := exec.Command("ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		alias, "true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh check for %s: %w: %s", alias, err, strings.TrimSpace(string(out)))
	}
	p.mu.Lock()
	p.intended[alias] = true
	p.mu.Unlock()
	return nil
}

func (p *procBackend) Disconnect(alias string) error {
	p.mu.Lock()
	procs := p.procs[alias]
	delete(p.procs, alias)
	delete(p.intended, alias)
	p.mu.Unlock()
	for _, cmd := range procs {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return nil
}

func (p *procBackend) Forward(alias string, port int) error {
	p.mu.Lock()
	if pp := p.procs[alias]; pp != nil {
		if _, exists := pp[port]; exists {
			p.mu.Unlock()
			return nil // already forwarding this port
		}
	}
	p.mu.Unlock()

	spec := fmt.Sprintf("%d:localhost:%d", port, port)
	cmd := exec.Command("ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=30",
		"-L", spec, alias)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ssh -L %s: %w", spec, err)
	}

	// Race: ssh exits early (forward setup failed) vs we time out and accept
	// it as up. The buffered channel + single Wait() avoids a double-Wait.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return fmt.Errorf("ssh -L %s on %s failed: %w: %s",
			spec, alias, err, strings.TrimSpace(stderr.String()))
	case <-time.After(1500 * time.Millisecond):
		// Still alive — assume the forward is up.
	}

	p.mu.Lock()
	if p.procs[alias] == nil {
		p.procs[alias] = map[int]*exec.Cmd{}
	}
	p.procs[alias][port] = cmd
	p.intended[alias] = true
	p.mu.Unlock()

	// Cleanup goroutine: when ssh eventually dies (network drop, host
	// reboot, explicit Cancel), drop it from our state. Reads the same
	// `done` channel that the wait goroutine writes to once.
	go func() {
		<-done
		p.mu.Lock()
		if pp := p.procs[alias]; pp != nil {
			if cur, ok := pp[port]; ok && cur == cmd {
				delete(pp, port)
			}
		}
		p.mu.Unlock()
	}()

	return nil
}

func (p *procBackend) Cancel(alias string, port int) error {
	p.mu.Lock()
	var cmd *exec.Cmd
	if pp := p.procs[alias]; pp != nil {
		cmd = pp[port]
		delete(pp, port)
	}
	p.mu.Unlock()
	if cmd == nil {
		return fmt.Errorf("no active forward for %s:%d", alias, port)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	// Cleanup goroutine started in Forward() will pick up cmd.Wait().
	return nil
}
