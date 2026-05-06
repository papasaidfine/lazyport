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
//
// PID is only meaningful for the procBackend (Windows / forced subprocess
// mode); it lets us re-adopt the underlying ssh process after lazyport
// restarts. Always 0 in masterBackend mode.
type Tunnel struct {
	Port int `json:"port"`
	PID  int `json:"pid,omitempty"`
}

// ResumedForward is what backend.Resume reports for each forward it has
// successfully adopted from a prior lazyport run.
type ResumedForward struct {
	Alias string
	Port  int
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

	// IsForwardActive reports whether a specific (host, port) forward is
	// currently up. masterBackend collapses to per-host IsConnected;
	// procBackend tracks per-process state.
	IsForwardActive(alias string, port int) bool

	// PID returns the OS process ID of the ssh subprocess maintaining the
	// forward, or 0 if there isn't one (masterBackend, or no such forward).
	PID(alias string, port int) int

	// Resume scans persisted state and adopts any forwards that are still
	// alive from a previous lazyport run. Returns the (alias, port) pairs
	// that were successfully re-tracked. Called once at startup, before the
	// UI is built.
	Resume(state *State) []ResumedForward
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
func IsConnected(alias string) bool                     { return activeBackend.IsConnected(alias) }
func Connect(alias string) error                        { return activeBackend.Connect(alias) }
func Disconnect(alias string) error                     { return activeBackend.Disconnect(alias) }
func Forward(alias string, port int) error              { return activeBackend.Forward(alias, port) }
func Cancel(alias string, port int) error               { return activeBackend.Cancel(alias, port) }
func IsForwardActive(alias string, port int) bool       { return activeBackend.IsForwardActive(alias, port) }
func ForwardPID(alias string, port int) int             { return activeBackend.PID(alias, port) }
func ResumeFromState(state *State) []ResumedForward     { return activeBackend.Resume(state) }

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

// In master mode the ControlMaster is the source of truth: if it's alive then
// every forward we recorded against it is alive too (ControlPersist=yes keeps
// per-forward state in the master). Per-forward queries collapse to per-host.
func (m *masterBackend) IsForwardActive(alias string, _ int) bool {
	return m.IsConnected(alias)
}

func (m *masterBackend) PID(_ string, _ int) int { return 0 }

func (m *masterBackend) Resume(state *State) []ResumedForward {
	if state == nil {
		return nil
	}
	var out []ResumedForward
	for alias, ts := range state.Hosts {
		if !m.IsConnected(alias) {
			continue
		}
		for _, t := range ts {
			out = append(out, ResumedForward{Alias: alias, Port: t.Port})
		}
	}
	return out
}

// ---------------- procBackend (one ssh per forward) ----------------

// forwardEntry tracks a single live forward. cmd is non-nil for forwards we
// spawned in this process; nil for forwards we adopted at startup (we only
// have the PID for those). pid is always populated.
type forwardEntry struct {
	cmd *exec.Cmd
	pid int
}

type procBackend struct {
	mu       sync.Mutex
	intended map[string]bool                   // hosts the user has clicked "connect" on
	procs    map[string]map[int]*forwardEntry  // alias → port → live forward
}

func newProcBackend() *procBackend {
	return &procBackend{
		intended: map[string]bool{},
		procs:    map[string]map[int]*forwardEntry{},
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
	for _, e := range procs {
		killEntry(e)
	}
	return nil
}

// killEntry kills the underlying process whether we spawned it (cmd) or
// adopted it (PID only).
func killEntry(e *forwardEntry) {
	if e == nil {
		return
	}
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		return
	}
	if e.pid > 0 {
		if proc, err := os.FindProcess(e.pid); err == nil {
			_ = proc.Kill()
		}
	}
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

	entry := &forwardEntry{cmd: cmd, pid: cmd.Process.Pid}

	p.mu.Lock()
	if p.procs[alias] == nil {
		p.procs[alias] = map[int]*forwardEntry{}
	}
	p.procs[alias][port] = entry
	p.intended[alias] = true
	p.mu.Unlock()

	// Cleanup goroutine: when ssh eventually dies (network drop, host
	// reboot, explicit Cancel), drop it from our state. Reads the same
	// `done` channel that the wait goroutine writes to once.
	go func() {
		<-done
		p.mu.Lock()
		if pp := p.procs[alias]; pp != nil {
			if cur, ok := pp[port]; ok && cur == entry {
				delete(pp, port)
			}
		}
		p.mu.Unlock()
	}()

	return nil
}

func (p *procBackend) Cancel(alias string, port int) error {
	p.mu.Lock()
	var entry *forwardEntry
	if pp := p.procs[alias]; pp != nil {
		entry = pp[port]
		delete(pp, port)
	}
	p.mu.Unlock()
	if entry == nil {
		return fmt.Errorf("no active forward for %s:%d", alias, port)
	}
	killEntry(entry)
	return nil
}

func (p *procBackend) IsForwardActive(alias string, port int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pp := p.procs[alias]; pp != nil {
		_, ok := pp[port]
		return ok
	}
	return false
}

func (p *procBackend) PID(alias string, port int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pp := p.procs[alias]; pp != nil {
		if e := pp[port]; e != nil {
			return e.pid
		}
	}
	return 0
}

// Resume re-adopts forwards from the persisted state whose PID is still alive
// and still belongs to an ssh process. PID-recycle protection: we check the
// process name matches "ssh" / "ssh.exe" so we don't mistakenly take over an
// unrelated process that was assigned the same PID after our previous run.
func (p *procBackend) Resume(state *State) []ResumedForward {
	if state == nil {
		return nil
	}
	var resumed []ResumedForward
	for alias, ts := range state.Hosts {
		anyAdopted := false
		for _, t := range ts {
			if t.PID <= 0 || !isSSHProcess(t.PID) {
				continue
			}
			p.mu.Lock()
			if p.procs[alias] == nil {
				p.procs[alias] = map[int]*forwardEntry{}
			}
			p.procs[alias][t.Port] = &forwardEntry{cmd: nil, pid: t.PID}
			p.mu.Unlock()
			resumed = append(resumed, ResumedForward{Alias: alias, Port: t.Port})
			anyAdopted = true
		}
		if anyAdopted {
			p.mu.Lock()
			p.intended[alias] = true
			p.mu.Unlock()
		}
	}
	return resumed
}

// isSSHProcess reports whether the given PID is currently an ssh process.
// Uses the per-OS facility for reading a process's image name; treats any
// failure as "not ssh" so we never adopt something we can't verify.
func isSSHProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	name, ok := processName(pid)
	if !ok {
		return false
	}
	name = strings.ToLower(name)
	return name == "ssh" || name == "ssh.exe"
}

func processName(pid int) (string, bool) {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(data)), true
	case "darwin":
		out, err := exec.Command("ps", "-o", "comm=", "-p", fmt.Sprintf("%d", pid)).Output()
		if err != nil {
			return "", false
		}
		s := strings.TrimSpace(string(out))
		if s == "" {
			return "", false
		}
		return filepath.Base(s), true
	case "windows":
		out, err := exec.Command("tasklist",
			"/FI", fmt.Sprintf("PID eq %d", pid),
			"/NH", "/FO", "CSV").Output()
		if err != nil {
			return "", false
		}
		s := strings.TrimSpace(string(out))
		if s == "" || strings.HasPrefix(s, "INFO:") {
			return "", false
		}
		// CSV row: "ssh.exe","12345","Console","1","X K"
		if idx := strings.Index(s, ","); idx > 0 {
			return strings.Trim(s[:idx], `"`), true
		}
		return "", false
	}
	return "", false
}
