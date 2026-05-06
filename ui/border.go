package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// titleBorder builds a top border line with the title baked into it:
//
//	╭─ Hosts ─────────╮
//
// paddedWidth is the same value passed to lipgloss.Style.Width() for the
// rest of the box — i.e. the *padded content* width, *not* the total visible
// width. Lipgloss adds 2 cells for the left+right border on top of Width(),
// and we mirror that here so the title row lines up with the body below it.
//
// Used together with style.BorderTop(false) on the rest of the box, this
// gives the panes the "sealed" header look from the original mockup without
// burning a content row on a separate title.
func titleBorder(paddedWidth int, title string, titleStyle, borderStyle lipgloss.Style) string {
	totalWidth := paddedWidth + 2 // L + R border
	titleW := lipgloss.Width(titleStyle.Render(title))
	// Layout: ╭─ <title> <fill ─...> ╮
	// Cell count: 1 (╭) + 1 (─) + 1 (space) + titleW + 1 (space) + fill + 1 (╮)
	//           = titleW + 5 + fill = totalWidth
	fill := totalWidth - titleW - 5
	if fill < 1 {
		// title too long — fall back to a plain top so we never overflow.
		return borderStyle.Render("╭" + strings.Repeat("─", totalWidth-2) + "╮")
	}
	return borderStyle.Render("╭─ ") +
		titleStyle.Render(title) +
		borderStyle.Render(" "+strings.Repeat("─", fill)+"╮")
}
