package ui

import (
	"errors"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PortInput is a numeric-only text field used to add a forward.
// It rejects non-digits at validation time so paste of "8080 " still works
// after a trim, while typed letters never make it into the buffer.
type PortInput struct {
	ti      textinput.Model
	focused bool
	width   int
}

func digitsOnly(s string) error {
	for _, r := range s {
		if r < '0' || r > '9' {
			return errors.New("digits only")
		}
	}
	return nil
}

func NewPortInput() PortInput {
	ti := textinput.New()
	ti.Placeholder = "port"
	ti.CharLimit = 5 // 65535
	ti.Prompt = ""
	ti.Validate = digitsOnly
	ti.Width = 8
	return PortInput{ti: ti}
}

func (p PortInput) Focused() bool { return p.focused }

func (p *PortInput) Focus() tea.Cmd {
	p.focused = true
	return p.ti.Focus()
}

func (p *PortInput) Blur() {
	p.focused = false
	p.ti.Blur()
}

func (p *PortInput) Reset() {
	p.ti.SetValue("")
}

// SetWidth lets the parent model resize the input as the layout changes.
func (p *PortInput) SetWidth(w int) {
	p.width = w
	// Keep some padding for the surrounding label and the focus border.
	tw := w - 14
	if tw < 6 {
		tw = 6
	}
	p.ti.Width = tw
}

// Submit returns a parsed valid port or 0 if the buffer is unusable.
// We treat 0 as "nothing to do" since unix port 0 isn't a meaningful target.
func (p PortInput) Submit() (int, bool) {
	v := strings.TrimSpace(p.ti.Value())
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > 65535 {
		return 0, false
	}
	return n, true
}

func (p PortInput) Update(msg tea.Msg) (PortInput, tea.Cmd) {
	if !p.focused {
		return p, nil
	}
	var cmd tea.Cmd
	p.ti, cmd = p.ti.Update(msg)
	return p, cmd
}

var (
	portLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	portBoxStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	portBoxFocusStyle = portBoxStyle.
				BorderForeground(lipgloss.Color("205"))
)

func (p PortInput) View() string {
	box := portBoxStyle
	if p.focused {
		box = portBoxFocusStyle
	}
	// The "press Enter to forward" hint used to live on this line, but it
	// overflowed past SetWidth's budget and bled into the box. The bottom
	// help bar already shows `enter forward · esc back` when this pane has
	// focus, so the inline hint was redundant.
	return lipgloss.JoinHorizontal(lipgloss.Center,
		portLabelStyle.Render("Add port: "),
		box.Render(p.ti.View()),
	)
}
