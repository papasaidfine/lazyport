package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HostItem is the minimal info the list needs to render a row.
// We intentionally don't pull in the parent package's Host type to keep ui/
// reusable and avoid a dependency cycle.
type HostItem struct {
	Alias     string
	Connected bool
}

// HostList is a focusable, navigable list of hosts shown in the left pane.
type HostList struct {
	items   []HostItem
	cursor  int
	focused bool
	width   int
	height  int
}

func NewHostList() HostList {
	return HostList{}
}

func (h *HostList) SetItems(items []HostItem) {
	h.items = items
	if h.cursor >= len(items) {
		h.cursor = len(items) - 1
	}
	if h.cursor < 0 {
		h.cursor = 0
	}
}

func (h HostList) Items() []HostItem { return h.items }
func (h HostList) Cursor() int       { return h.cursor }

// Selected returns the item under the cursor, or zero-value if the list is empty.
func (h HostList) Selected() (HostItem, bool) {
	if len(h.items) == 0 {
		return HostItem{}, false
	}
	return h.items[h.cursor], true
}

func (h *HostList) Focus()         { h.focused = true }
func (h *HostList) Blur()          { h.focused = false }
func (h HostList) Focused() bool   { return h.focused }
func (h *HostList) SetSize(w, hgt int) {
	h.width, h.height = w, hgt
}

// Update handles vertical navigation. The parent owns Enter (connect/disconnect)
// since that requires running an external command.
func (h HostList) Update(msg tea.Msg) (HostList, tea.Cmd) {
	if !h.focused {
		return h, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if h.cursor > 0 {
				h.cursor--
			}
		case "down", "j":
			if h.cursor < len(h.items)-1 {
				h.cursor++
			}
		case "home", "g":
			h.cursor = 0
		case "end", "G":
			h.cursor = len(h.items) - 1
			if h.cursor < 0 {
				h.cursor = 0
			}
		}
	}
	return h, nil
}

var (
	hostsPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	hostsPaneFocusStyle = hostsPaneStyle.
				BorderForeground(lipgloss.Color("205"))

	hostsTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	hostRowSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("57")).
				Bold(true)

	hostRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	hostMutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
)

func (h HostList) View() string {
	style := hostsPaneStyle
	if h.focused {
		style = hostsPaneFocusStyle
	}
	style = style.Width(h.width).Height(h.height)

	var b strings.Builder
	b.WriteString(hostsTitleStyle.Render("Hosts"))
	b.WriteString("\n\n")

	if len(h.items) == 0 {
		b.WriteString(hostMutedStyle.Render("(no hosts in ~/.ssh/config)"))
		return style.Render(b.String())
	}

	innerW := h.width - 4 // borders + padding
	if innerW < 10 {
		innerW = 10
	}

	for i, it := range h.items {
		dot := "○"
		dotStyle := hostMutedStyle
		if it.Connected {
			dot = "●"
			dotStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		}
		marker := "  "
		if i == h.cursor {
			marker = "> "
		}

		nameW := innerW - len(marker) - 2
		if nameW < 1 {
			nameW = 1
		}
		name := truncate(it.Alias, nameW)
		row := marker + padRight(name, nameW) + " " + dotStyle.Render(dot)

		if i == h.cursor && h.focused {
			row = hostRowSelectedStyle.Render(row)
		} else {
			row = hostRowStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteByte('\n')
	}
	return style.Render(strings.TrimRight(b.String(), "\n"))
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
