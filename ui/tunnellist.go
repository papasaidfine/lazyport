package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TunnelItem is a row in the right pane: one forwarded port and its status.
//
// A row falls into one of three states:
//   - Active: forward is currently up.
//   - Stopped: user explicitly toggled it off; can be resumed.
//   - neither: forward is down for reasons other than the user pausing it
//     (process died, host disconnected, never spawned). Shown as "down".
type TunnelItem struct {
	Port    int
	Active  bool
	Stopped bool
	Note    string // optional message (e.g. last error)
}

// TunnelList renders the right pane: title, a list of tunnels, and the
// embedded port input. The pane has two focus modes — the list (where
// j/k/d/Enter act on rows) and the input (typing a port).
type TunnelList struct {
	hostAlias string
	connected bool
	items     []TunnelItem
	cursor    int

	focusedList  bool // list rows are focused
	width        int
	height       int

	input PortInput
}

func NewTunnelList() TunnelList {
	return TunnelList{input: NewPortInput()}
}

func (t *TunnelList) SetHost(alias string, connected bool) {
	t.hostAlias = alias
	t.connected = connected
}

func (t *TunnelList) SetItems(items []TunnelItem) {
	t.items = items
	if t.cursor >= len(items) {
		t.cursor = len(items) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

func (t TunnelList) Items() []TunnelItem { return t.items }
func (t TunnelList) Cursor() int         { return t.cursor }

func (t TunnelList) Selected() (TunnelItem, bool) {
	if len(t.items) == 0 {
		return TunnelItem{}, false
	}
	return t.items[t.cursor], true
}

// FocusList puts focus on the list rows. The port input is always given its
// own focus state via FocusInput so the parent can switch between the two.
func (t *TunnelList) FocusList() {
	t.focusedList = true
	t.input.Blur()
}

func (t *TunnelList) FocusInput() tea.Cmd {
	t.focusedList = false
	return t.input.Focus()
}

func (t *TunnelList) Blur() {
	t.focusedList = false
	t.input.Blur()
}

func (t TunnelList) ListFocused() bool  { return t.focusedList }
func (t TunnelList) InputFocused() bool { return t.input.Focused() }
func (t TunnelList) AnyFocused() bool   { return t.focusedList || t.input.Focused() }

func (t *TunnelList) ResetInput() {
	t.input.Reset()
}

func (t TunnelList) SubmitInput() (int, bool) {
	return t.input.Submit()
}

func (t *TunnelList) SetSize(w, h int) {
	t.width, t.height = w, h
	t.input.SetWidth(w - 4)
}

// Update handles only navigation within the tunnel list and forwards
// keystrokes to the port input when it's focused. Action keys (Enter to add,
// 'd' to delete) bubble up through the parent so it can run external commands.
func (t TunnelList) Update(msg tea.Msg) (TunnelList, tea.Cmd) {
	if t.focusedList {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "up", "k":
				if t.cursor > 0 {
					t.cursor--
				}
			case "down", "j":
				if t.cursor < len(t.items)-1 {
					t.cursor++
				}
			}
		}
		return t, nil
	}
	if t.input.Focused() {
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}
	return t, nil
}

var (
	tunnelsPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
	tunnelsPaneFocusStyle = tunnelsPaneStyle.
				BorderForeground(lipgloss.Color("205"))

	tunnelsTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Underline(true)

	tunnelRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	tunnelRowSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("57")).
				Bold(true)

	statusActiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusDownStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	statusPausedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	delHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	hintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func (t TunnelList) View() string {
	style := tunnelsPaneStyle
	if t.AnyFocused() {
		style = tunnelsPaneFocusStyle
	}
	style = style.Width(t.width).Height(t.height)

	var b strings.Builder
	title := "Tunnels"
	if t.hostAlias != "" {
		title = "Tunnels: " + t.hostAlias
		if !t.connected {
			title += "  " + hintStyle.Render("(disconnected)")
		}
	}
	b.WriteString(tunnelsTitleStyle.Render(title))
	b.WriteString("\n\n")

	if t.hostAlias == "" {
		b.WriteString(hintStyle.Render("Pick a host on the left, press Enter to connect."))
		return style.Render(b.String())
	}

	// header
	b.WriteString(tableHeaderStyle.Render(fmt.Sprintf("%-7s %-10s %s", "PORT", "STATUS", "")))
	b.WriteByte('\n')

	if len(t.items) == 0 {
		b.WriteString(hintStyle.Render("(no forwards yet — type a port below)"))
		b.WriteByte('\n')
	} else {
		for i, it := range t.items {
			var status string
			switch {
			case it.Active:
				status = statusActiveStyle.Render("active")
			case it.Stopped:
				status = statusPausedStyle.Render("paused")
			default:
				status = statusDownStyle.Render("down  ")
			}
			row := fmt.Sprintf("%-7d %s   %s",
				it.Port, status, delHintStyle.Render("[del]"))
			if i == t.cursor && t.focusedList {
				row = tunnelRowSelectedStyle.Render(row)
			} else {
				row = tunnelRowStyle.Render(row)
			}
			b.WriteString(row)
			b.WriteByte('\n')
		}
	}

	// Spacer to push the input to the bottom-ish.
	// We don't try to perfectly bottom-align — keeps layout robust under resize.
	b.WriteByte('\n')
	b.WriteString(t.input.View())

	if t.focusedList && len(t.items) > 0 {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("space  pause/resume    d / x  delete    Tab  switch pane"))
	}

	return style.Render(b.String())
}
