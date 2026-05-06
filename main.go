package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/papasaidfine/lazyport/ui"
)

// Build-time metadata, populated via -ldflags by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	themeFlag := flag.String("theme", "",
		"color theme — one of: "+strings.Join(ui.ThemeNames(), ", ")+
			" (default: nord; can also be set via $LAZYPORT_THEME)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("lazyport %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	// CLI flag wins over env var; both are optional and fall back to Nord.
	themeName := *themeFlag
	if themeName == "" {
		themeName = os.Getenv("LAZYPORT_THEME")
	}
	if err := ui.SetTheme(themeName); err != nil {
		fmt.Fprintf(os.Stderr, "lazyport: %v\n", err)
		os.Exit(1)
	}

	hosts, err := ListHosts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lazyport: read ssh config: %v\n", err)
		os.Exit(1)
	}
	state, err := LoadState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lazyport: load state: %v\n", err)
		os.Exit(1)
	}

	if !SupportsControlMaster() {
		fmt.Fprintln(os.Stderr,
			"lazyport: using per-forward ssh subprocesses (no ControlMaster on this platform).")
	}

	m := NewModel(hosts, state)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lazyport: %v\n", err)
		os.Exit(1)
	}
}
