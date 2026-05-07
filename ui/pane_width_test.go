package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Regression test for the "right border missing" bug.
//
// model.layout() splits the screen as `leftW + 1 (spacer) + rightW = m.width`
// and passes leftW/rightW into pane.SetSize. The panes must therefore render
// at exactly that width *visibly* — including their L+R rounded border. Prior
// to this fix, the panes called lipgloss.Style.Width(h.width), which renders
// (h.width + 2) visible cells, blowing the row over m.width by 4 cells and
// clipping the right border off the right pane.
func TestPaneVisibleWidthMatchesRequest(t *testing.T) {
	const requested = 30

	hl := NewHostList()
	hl.SetSize(requested, 6)
	hl.SetItems([]HostItem{{Alias: "dev"}, {Alias: "prod", Connected: true}})
	if got := maxLineWidth(hl.View()); got != requested {
		t.Errorf("HostList visible width = %d, want %d", got, requested)
	}

	tl := NewTunnelList()
	tl.SetSize(requested, 6)
	tl.SetHost("dev", true)
	tl.SetItems([]TunnelItem{{Port: 8080, Active: true}})
	if got := maxLineWidth(tl.View()); got != requested {
		t.Errorf("TunnelList visible width = %d, want %d", got, requested)
	}
}

func maxLineWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > max {
			max = w
		}
	}
	return max
}
