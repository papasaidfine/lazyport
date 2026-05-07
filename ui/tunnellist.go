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

func (t TunnelList) View() string {
	th := ActiveTheme

	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		Padding(0, 1)
	if t.AnyFocused() {
		paneStyle = paneStyle.BorderForeground(th.BorderFocused)
	}
	titleStyle := lipgloss.NewStyle().Foreground(th.Title).Bold(true)
	tableHeaderStyle := lipgloss.NewStyle().Foreground(th.TextMuted).Underline(true)
	rowStyle := lipgloss.NewStyle().Foreground(th.Text)
	rowSelectedStyle := lipgloss.NewStyle().
		Foreground(th.TextSelected).
		Background(th.BgSelected).
		Bold(true)
	statusActiveStyle := lipgloss.NewStyle().Foreground(th.StatusActive)
	statusDownStyle := lipgloss.NewStyle().Foreground(th.StatusDown)
	statusPausedStyle := lipgloss.NewStyle().Foreground(th.StatusPaused)
	mutedStyle := lipgloss.NewStyle().Foreground(th.TextMuted)
	borderColor := th.Border
	if t.AnyFocused() {
		borderColor = th.BorderFocused
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// lipgloss adds the L/R borders *on top of* Width() and the bottom border
	// on top of Height(), so we shrink both by 2 to land at exactly the
	// visible t.width × t.height the layout asked for.
	padded := t.width - 2
	style := paneStyle.BorderTop(false).Width(padded).Height(t.height - 2)

	title := "Tunnels"
	if t.hostAlias != "" {
		title = "Tunnels: " + t.hostAlias
	}
	header := titleBorder(padded, title, titleStyle, borderStyle)

	var b strings.Builder
	if t.hostAlias == "" {
		b.WriteString(mutedStyle.Render("Pick a host on the left, press Enter to connect."))
		return header + "\n" + style.Render(b.String())
	}

	// header
	b.WriteString(tableHeaderStyle.Render(fmt.Sprintf("%-7s %-10s %s", "PORT", "STATUS", "")))
	b.WriteByte('\n')

	if len(t.items) == 0 {
		b.WriteString(mutedStyle.Render("(no forwards yet — type a port below)"))
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
				it.Port, status, mutedStyle.Render("[del]"))
			if i == t.cursor && t.focusedList {
				row = rowSelectedStyle.Render(row)
			} else {
				row = rowStyle.Render(row)
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
		b.WriteString(mutedStyle.Render("space  pause/resume    d / x  delete    Tab  switch pane"))
	}

	return header + "\n" + style.Render(b.String())
}
