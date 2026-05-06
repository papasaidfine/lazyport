package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lchen/sshfwd/ui"
)

// focusZone is which pane currently owns input.
type focusZone int

const (
	focusHosts focusZone = iota
	focusTunnels
	focusInput
)

// quitStage tracks the small "keep tunnels alive?" modal shown on Ctrl+C / q.
type quitStage int

const (
	quitInactive quitStage = iota
	quitAsking
)

// Messages used to communicate the result of an external `ssh` command back
// into the bubbletea Update loop. Each carries the alias it concerns so a
// later, slower command can't clobber an unrelated host's state.
type (
	connectedMsg    struct{ alias string; err error }
	disconnectedMsg struct{ alias string; err error }
	forwardedMsg    struct{ alias string; port int; err error }
	canceledMsg     struct{ alias string; port int; err error }
	tickMsg         time.Time
	statusMsg       struct{ text string; err bool }
)

// Model is the top-level bubbletea model for sshfwd.
type Model struct {
	hosts     []Host
	connected map[string]bool
	tunnels   map[string][]Tunnel // per-host tunnel list (mirror of state)
	state     *State

	hostList   ui.HostList
	tunnelList ui.TunnelList

	focus focusZone

	width  int
	height int

	statusText string
	statusIsErr bool

	quitStage quitStage

	// pending tracks per-(host,port) work in flight, so we don't double-queue.
	pendingForward map[string]map[int]bool
}

func NewModel(hosts []Host, state *State) Model {
	m := Model{
		hosts:          hosts,
		connected:      map[string]bool{},
		tunnels:        map[string][]Tunnel{},
		state:          state,
		hostList:       ui.NewHostList(),
		tunnelList:     ui.NewTunnelList(),
		focus:          focusHosts,
		pendingForward: map[string]map[int]bool{},
	}
	for _, h := range hosts {
		m.connected[h.Alias] = IsConnected(h.Alias)
		if t := state.Get(h.Alias); len(t) > 0 {
			m.tunnels[h.Alias] = t
		}
	}
	m.refreshHostList()
	m.refreshTunnelList()
	m.hostList.Focus()
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, tickEvery())
}

// tickEvery refreshes ControlMaster status periodically so the dot stays
// honest even if a master dies behind our back.
func tickEvery() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) refreshHostList() {
	items := make([]ui.HostItem, 0, len(m.hosts))
	for _, h := range m.hosts {
		items = append(items, ui.HostItem{Alias: h.Alias, Connected: m.connected[h.Alias]})
	}
	m.hostList.SetItems(items)
}

func (m *Model) refreshTunnelList() {
	sel, ok := m.hostList.Selected()
	if !ok {
		m.tunnelList.SetHost("", false)
		m.tunnelList.SetItems(nil)
		return
	}
	connected := m.connected[sel.Alias]
	m.tunnelList.SetHost(sel.Alias, connected)

	tunnels := m.tunnels[sel.Alias]
	items := make([]ui.TunnelItem, 0, len(tunnels))
	for _, t := range tunnels {
		items = append(items, ui.TunnelItem{Port: t.Port, Active: connected})
	}
	m.tunnelList.SetItems(items)
}

func (m *Model) saveState() {
	if m.state == nil {
		return
	}
	for alias, ts := range m.tunnels {
		m.state.Set(alias, ts)
	}
	if err := m.state.Save(); err != nil {
		m.setStatus("save state: "+err.Error(), true)
	}
}

func (m *Model) setStatus(s string, isErr bool) {
	m.statusText = s
	m.statusIsErr = isErr
}

// markPending / clearPending guard against double-submission when ssh is slow.
func (m *Model) markPending(alias string, port int) bool {
	mm := m.pendingForward[alias]
	if mm == nil {
		mm = map[int]bool{}
		m.pendingForward[alias] = mm
	}
	if mm[port] {
		return false
	}
	mm[port] = true
	return true
}

func (m *Model) clearPending(alias string, port int) {
	if mm := m.pendingForward[alias]; mm != nil {
		delete(mm, port)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tickMsg:
		// Refresh ControlMaster liveness for each host.
		changed := false
		for _, h := range m.hosts {
			c := IsConnected(h.Alias)
			if c != m.connected[h.Alias] {
				m.connected[h.Alias] = c
				changed = true
			}
		}
		if changed {
			m.refreshHostList()
			m.refreshTunnelList()
		}
		return m, tickEvery()

	case connectedMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("connect %s: %v", msg.alias, msg.err), true)
		} else {
			m.connected[msg.alias] = true
			m.setStatus("connected to "+msg.alias, false)
			// Re-establish any persisted tunnels for this host.
			for _, t := range m.tunnels[msg.alias] {
				p := t.Port
				alias := msg.alias
				if !m.markPending(alias, p) {
					continue
				}
				return m.afterUpdate(forwardCmd(alias, p))
			}
		}
		m.refreshHostList()
		m.refreshTunnelList()
		return m, nil

	case disconnectedMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("disconnect %s: %v", msg.alias, msg.err), true)
		} else {
			m.setStatus("disconnected "+msg.alias, false)
		}
		m.connected[msg.alias] = false
		m.refreshHostList()
		m.refreshTunnelList()
		return m, nil

	case forwardedMsg:
		m.clearPending(msg.alias, msg.port)
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("forward %d on %s: %v", msg.port, msg.alias, msg.err), true)
			// Don't add to tunnels list if it didn't take.
		} else {
			m.addTunnel(msg.alias, msg.port)
			m.setStatus(fmt.Sprintf("forwarded :%d → %s:%d", msg.port, msg.alias, msg.port), false)
			m.saveState()
		}
		m.refreshTunnelList()
		return m, nil

	case canceledMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("cancel %d on %s: %v", msg.port, msg.alias, msg.err), true)
		} else {
			m.removeTunnel(msg.alias, msg.port)
			m.setStatus(fmt.Sprintf("removed :%d on %s", msg.port, msg.alias), false)
			m.saveState()
		}
		m.refreshTunnelList()
		return m, nil

	case statusMsg:
		m.setStatus(msg.text, msg.err)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// afterUpdate is a small helper so message handlers can return both a status
// refresh and a command in one line.
func (m Model) afterUpdate(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	return m, cmd
}

func (m *Model) addTunnel(alias string, port int) {
	for _, t := range m.tunnels[alias] {
		if t.Port == port {
			return
		}
	}
	m.tunnels[alias] = append(m.tunnels[alias], Tunnel{Port: port})
}

func (m *Model) removeTunnel(alias string, port int) {
	src := m.tunnels[alias]
	out := src[:0]
	for _, t := range src {
		if t.Port != port {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		delete(m.tunnels, alias)
	} else {
		m.tunnels[alias] = out
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Quit modal takes precedence.
	if m.quitStage == quitAsking {
		switch msg.String() {
		case "k", "K":
			// Keep tunnels alive: don't tear down masters.
			m.saveState()
			return m, tea.Quit
		case "t", "T", "y", "Y":
			// Tear down all masters.
			m.saveState()
			cmds := []tea.Cmd{}
			for _, h := range m.hosts {
				if m.connected[h.Alias] {
					alias := h.Alias
					cmds = append(cmds, func() tea.Msg {
						_ = Disconnect(alias)
						return nil
					})
				}
			}
			cmds = append(cmds, tea.Quit)
			return m, tea.Sequence(cmds...)
		case "esc", "n", "N":
			m.quitStage = quitInactive
			m.setStatus("", false)
			return m, nil
		}
		return m, nil
	}

	// While the port input is focused, only Enter, Esc, and Tab are special;
	// everything else goes to the input.
	if m.focus == focusInput {
		switch msg.String() {
		case "esc":
			m.tunnelList.Blur()
			m.focus = focusTunnels
			m.tunnelList.FocusList()
			return m, nil
		case "tab", "shift+tab":
			m.tunnelList.Blur()
			m.focus = focusHosts
			m.hostList.Focus()
			return m, nil
		case "enter":
			port, ok := m.tunnelList.SubmitInput()
			if !ok {
				m.setStatus("invalid port", true)
				return m, nil
			}
			sel, hasSel := m.hostList.Selected()
			if !hasSel {
				m.setStatus("no host selected", true)
				return m, nil
			}
			if !m.markPending(sel.Alias, port) {
				return m, nil
			}
			m.tunnelList.ResetInput()
			return m, forwardCmd(sel.Alias, port)
		}
		var cmd tea.Cmd
		m.tunnelList, cmd = m.tunnelList.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c", "q":
		// If nothing is connected, just quit cleanly.
		anyConnected := false
		for _, c := range m.connected {
			if c {
				anyConnected = true
				break
			}
		}
		if !anyConnected {
			m.saveState()
			return m, tea.Quit
		}
		m.quitStage = quitAsking
		return m, nil

	case "tab":
		return m.cycleFocus(true)
	case "shift+tab":
		return m.cycleFocus(false)
	case "left", "h":
		if m.focus == focusTunnels {
			m.focus = focusHosts
			m.tunnelList.Blur()
			m.hostList.Focus()
		}
		return m, nil
	case "right", "l":
		if m.focus == focusHosts {
			m.focus = focusTunnels
			m.hostList.Blur()
			m.tunnelList.FocusList()
		}
		return m, nil
	case "i", "/":
		// Quick jump to port input.
		m.hostList.Blur()
		m.tunnelList.Blur()
		m.focus = focusInput
		return m, m.tunnelList.FocusInput()
	}

	switch m.focus {
	case focusHosts:
		return m.handleHostsKey(msg)
	case focusTunnels:
		return m.handleTunnelsKey(msg)
	}
	return m, nil
}

func (m Model) cycleFocus(forward bool) (tea.Model, tea.Cmd) {
	order := []focusZone{focusHosts, focusTunnels, focusInput}
	idx := 0
	for i, f := range order {
		if f == m.focus {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(order)
	} else {
		idx = (idx - 1 + len(order)) % len(order)
	}
	m.hostList.Blur()
	m.tunnelList.Blur()
	m.focus = order[idx]
	switch m.focus {
	case focusHosts:
		m.hostList.Focus()
		return m, nil
	case focusTunnels:
		m.tunnelList.FocusList()
		return m, nil
	case focusInput:
		return m, m.tunnelList.FocusInput()
	}
	return m, nil
}

func (m Model) handleHostsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		sel, ok := m.hostList.Selected()
		if !ok {
			return m, nil
		}
		if m.connected[sel.Alias] {
			return m, disconnectCmd(sel.Alias)
		}
		return m, connectCmd(sel.Alias)
	}
	var cmd tea.Cmd
	m.hostList, cmd = m.hostList.Update(msg)
	m.refreshTunnelList()
	return m, cmd
}

func (m Model) handleTunnelsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Enter on the list either jumps to input (if empty list) or has no
		// effect on a selected row. Most-used path is to jump to input.
		m.tunnelList.Blur()
		m.focus = focusInput
		return m, m.tunnelList.FocusInput()
	case "d", "x", "delete", "backspace":
		sel, ok := m.tunnelList.Selected()
		if !ok {
			return m, nil
		}
		host, hostOk := m.hostList.Selected()
		if !hostOk {
			return m, nil
		}
		return m, cancelCmd(host.Alias, sel.Port)
	}
	var cmd tea.Cmd
	m.tunnelList, cmd = m.tunnelList.Update(msg)
	return m, cmd
}

func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	leftW := 24
	if leftW > m.width/3 {
		leftW = m.width / 3
		if leftW < 16 {
			leftW = 16
		}
	}
	rightW := m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	contentH := m.height - 2 // status bar + a bit of breathing room
	if contentH < 8 {
		contentH = 8
	}
	m.hostList.SetSize(leftW, contentH)
	m.tunnelList.SetSize(rightW, contentH)
}

var (
	statusOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(1, 2)
)

func (m Model) View() string {
	if m.width == 0 {
		return "starting up…"
	}

	if m.quitStage == quitAsking {
		body := "Some tunnels are still active.\n\n" +
			"  [k] keep them running in the background\n" +
			"  [t] tear them all down and exit\n" +
			"  [esc] cancel"
		return modalStyle.Render(body)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, m.hostList.View(), " ", m.tunnelList.View())

	status := m.statusLine()
	help := helpStyle.Render(m.helpLine())

	return lipgloss.JoinVertical(lipgloss.Left, body, status, help)
}

func (m Model) statusLine() string {
	if m.statusText == "" {
		return ""
	}
	if m.statusIsErr {
		return statusErrStyle.Render("✖ " + m.statusText)
	}
	return statusOKStyle.Render("✓ " + m.statusText)
}

func (m Model) helpLine() string {
	parts := []string{
		"tab/←→ pane",
		"↑↓ navigate",
	}
	switch m.focus {
	case focusHosts:
		parts = append(parts, "enter connect/disconnect")
	case focusTunnels:
		parts = append(parts, "d delete", "i add port")
	case focusInput:
		parts = append(parts, "enter forward", "esc back")
	}
	parts = append(parts, "q quit")
	return strings.Join(parts, "  ·  ")
}

// --- ssh command wrappers as tea.Cmd ---

func connectCmd(alias string) tea.Cmd {
	return func() tea.Msg {
		err := Connect(alias)
		return connectedMsg{alias: alias, err: err}
	}
}

func disconnectCmd(alias string) tea.Cmd {
	return func() tea.Msg {
		err := Disconnect(alias)
		return disconnectedMsg{alias: alias, err: err}
	}
}

func forwardCmd(alias string, port int) tea.Cmd {
	return func() tea.Msg {
		err := Forward(alias, port)
		return forwardedMsg{alias: alias, port: port, err: err}
	}
}

func cancelCmd(alias string, port int) tea.Cmd {
	return func() tea.Msg {
		err := Cancel(alias, port)
		return canceledMsg{alias: alias, port: port, err: err}
	}
}
