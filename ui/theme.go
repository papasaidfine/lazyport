package ui

import "github.com/charmbracelet/lipgloss"

// Nord — https://www.nordtheme.com/
//
// Lipgloss downgrades these to the nearest 256-color palette match on
// terminals that don't support truecolor, so the look degrades gracefully on
// legacy cmd.exe / older ConHost.
var (
	// Polar Night — backgrounds and muted UI chrome.
	nord0 = lipgloss.Color("#2E3440")
	nord1 = lipgloss.Color("#3B4252")
	nord2 = lipgloss.Color("#434C5E")
	nord3 = lipgloss.Color("#4C566A")

	// Snow Storm — foreground / regular text.
	nord4 = lipgloss.Color("#D8DEE9")
	nord5 = lipgloss.Color("#E5E9F0")
	nord6 = lipgloss.Color("#ECEFF4")

	// Frost — primary accents (focus, titles, "current" state).
	nord7  = lipgloss.Color("#8FBCBB")
	nord8  = lipgloss.Color("#88C0D0")
	nord9  = lipgloss.Color("#81A1C1")
	nord10 = lipgloss.Color("#5E81AC")

	// Aurora — semantic colors.
	nord11 = lipgloss.Color("#BF616A") // red — error / down
	nord12 = lipgloss.Color("#D08770") // orange — warning (unused)
	nord13 = lipgloss.Color("#EBCB8B") // yellow — paused / hold
	nord14 = lipgloss.Color("#A3BE8C") // green — active / success
	nord15 = lipgloss.Color("#B48EAD") // purple — special (unused)
)

// Project-level semantic aliases. Tweaking the look-and-feel of lazyport
// should mostly happen here, not at every call site. Exported entries are
// used from the main package (status bar, quit modal).
var (
	colorBorder        = nord3
	colorBorderFocused = nord8
	colorTitle         = nord8
	colorText          = nord4
	colorTextMuted     = nord3
	colorTextSelected  = nord6
	colorBgSelected    = nord10

	colorStatusActive = nord14
	colorStatusDown   = nord11
	colorStatusPaused = nord13
)

// Exported names for use from the main package.
var (
	ColorBorderFocused = colorBorderFocused
	ColorTextMuted     = colorTextMuted
	ColorStatusActive  = colorStatusActive
	ColorStatusDown    = colorStatusDown
)

// Suppress "declared but unused" complaints for the unused palette entries —
// we keep them around so future additions can pull the right shade without
// re-deriving it from the website.
var _ = []lipgloss.Color{nord0, nord1, nord2, nord5, nord7, nord9, nord12, nord15}
