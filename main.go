package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Build-time metadata, populated via -ldflags by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sshfwd %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	hosts, err := ListHosts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshfwd: read ssh config: %v\n", err)
		os.Exit(1)
	}
	state, err := LoadState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshfwd: load state: %v\n", err)
		os.Exit(1)
	}

	if !SupportsControlMaster() {
		fmt.Fprintln(os.Stderr,
			"sshfwd: warning — ControlMaster multiplexing is not supported on this OS;\n"+
				"        port forwards will be added without a long-lived control socket.")
	}

	m := NewModel(hosts, state)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sshfwd: %v\n", err)
		os.Exit(1)
	}
}
