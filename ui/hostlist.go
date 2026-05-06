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

// Styles are built per-render from ActiveTheme so a theme switch (CLI flag,
// or — should we add it later — a hot-key) is reflected immediately. lipgloss
// styles are cheap structs; allocating ~6 of them per frame is fine.
func (h HostList) View() string {
	t := ActiveTheme
	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)
	if h.focused {
		paneStyle = paneStyle.BorderForeground(t.BorderFocused)
	}
	titleStyle := lipgloss.NewStyle().Foreground(t.Title).Bold(true)
	rowSelectedStyle := lipgloss.NewStyle().
		Foreground(t.TextSelected).
		Background(t.BgSelected).
		Bold(true)
	rowStyle := lipgloss.NewStyle().Foreground(t.Text)
	mutedStyle := lipgloss.NewStyle().Foreground(t.TextMuted)
	borderColor := t.Border
	if h.focused {
		borderColor = t.BorderFocused
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Render the box with no top border; we'll prepend our own title-bearing
	// top so the title sits on the border line itself rather than below it.
	// Height in lipgloss is content rows; borders are added on top. We render
	// 1 manual top + N content + 1 bottom = N + 2 = h.height rows.
	style := paneStyle.BorderTop(false).Width(h.width).Height(h.height - 2)

	var b strings.Builder

	if len(h.items) == 0 {
		b.WriteString(mutedStyle.Render("(no hosts in ~/.ssh/config)"))
		body := style.Render(b.String())
		return titleBorder(h.width, "Hosts", titleStyle, borderStyle) + "\n" + body
	}

	innerW := h.width - 4 // borders + padding
	if innerW < 10 {
		innerW = 10
	}

	for i, it := range h.items {
		dot := "○"
		dotStyle := mutedStyle
		if it.Connected {
			dot = "●"
			dotStyle = lipgloss.NewStyle().Foreground(t.StatusActive)
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
			row = rowSelectedStyle.Render(row)
		} else {
			row = rowStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteByte('\n')
	}
	body := style.Render(strings.TrimRight(b.String(), "\n"))
	return titleBorder(h.width, "Hosts", titleStyle, borderStyle) + "\n" + body
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
