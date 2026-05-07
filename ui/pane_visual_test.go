package ui

import (
	"strings"
	"testing"
)

// Defense-in-depth: also assert the bottom-right corner glyph is present in
// the rendered output. A clipped right border (the bug we just fixed) would
// drop the ╯ from the last line, even on a terminal that wraps gracefully.
func TestPaneBottomRightCornerPresent(t *testing.T) {
	hl := NewHostList()
	hl.SetSize(28, 6)
	hl.SetItems([]HostItem{{Alias: "dev"}})
	lines := strings.Split(hl.View(), "\n")
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, "╯") {
		t.Errorf("HostList bottom row missing right corner ╯: %q", last)
	}

	tl := NewTunnelList()
	tl.SetSize(40, 8)
	tl.SetHost("dev", true)
	tl.SetItems([]TunnelItem{{Port: 8080, Active: true}})
	lines = strings.Split(tl.View(), "\n")
	last = lines[len(lines)-1]
	if !strings.HasSuffix(last, "╯") {
		t.Errorf("TunnelList bottom row missing right corner ╯: %q", last)
	}
}
