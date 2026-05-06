package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is the bag of colors lazyport's UI reads at render time.
//
// Styles are intentionally NOT cached in package-level vars: the active
// theme can change between View() calls (CLI flag at startup, or — should we
// add it later — a hot-key), and lipgloss styles are cheap enough to build
// inline in each render path.
type Theme struct {
	Name string

	Border        lipgloss.Color // muted pane border
	BorderFocused lipgloss.Color // pane border when the pane has focus
	Title         lipgloss.Color // header text baked into the top border

	Text         lipgloss.Color // primary body text
	TextMuted    lipgloss.Color // hints, table headers, faded UI chrome
	TextSelected lipgloss.Color // foreground of the selected row
	BgSelected   lipgloss.Color // background of the selected row

	StatusActive lipgloss.Color // green — running forward / connected dot
	StatusDown   lipgloss.Color // red   — error / unexpectedly down
	StatusPaused lipgloss.Color // yellow — user-paused forward
}

// Nord — https://www.nordtheme.com/
var Nord = Theme{
	Name:          "nord",
	Border:        lipgloss.Color("#4C566A"),
	BorderFocused: lipgloss.Color("#88C0D0"),
	Title:         lipgloss.Color("#88C0D0"),
	Text:          lipgloss.Color("#D8DEE9"),
	TextMuted:     lipgloss.Color("#4C566A"),
	TextSelected:  lipgloss.Color("#ECEFF4"),
	BgSelected:    lipgloss.Color("#5E81AC"),
	StatusActive:  lipgloss.Color("#A3BE8C"),
	StatusDown:    lipgloss.Color("#BF616A"),
	StatusPaused:  lipgloss.Color("#EBCB8B"),
}

// Dracula — https://draculatheme.com/
var Dracula = Theme{
	Name:          "dracula",
	Border:        lipgloss.Color("#6272A4"),
	BorderFocused: lipgloss.Color("#BD93F9"),
	Title:         lipgloss.Color("#FF79C6"),
	Text:          lipgloss.Color("#F8F8F2"),
	TextMuted:     lipgloss.Color("#6272A4"),
	TextSelected:  lipgloss.Color("#F8F8F2"),
	BgSelected:    lipgloss.Color("#44475A"),
	StatusActive:  lipgloss.Color("#50FA7B"),
	StatusDown:    lipgloss.Color("#FF5555"),
	StatusPaused:  lipgloss.Color("#F1FA8C"),
}

// Themes is the registry surfaced to the CLI for --theme.
var Themes = map[string]Theme{
	"nord":    Nord,
	"dracula": Dracula,
}

// ActiveTheme is the live palette that View() functions read on each render.
// Default is Nord.
var ActiveTheme = Nord

// SetTheme switches the active palette by name. An empty name is a no-op
// (callers can pass an unset env var directly without checking).
func SetTheme(name string) error {
	if name == "" {
		return nil
	}
	t, ok := Themes[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("unknown theme %q (available: %s)",
			name, strings.Join(ThemeNames(), ", "))
	}
	ActiveTheme = t
	return nil
}

// ThemeNames returns the registered theme names in stable order — useful for
// CLI help text and error messages.
func ThemeNames() []string {
	names := make([]string, 0, len(Themes))
	for n := range Themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
