package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ChristopherBilg/lazygh/internal/config"
	"github.com/ChristopherBilg/lazygh/internal/github"
	"github.com/ChristopherBilg/lazygh/internal/logging"
	"github.com/ChristopherBilg/lazygh/internal/tui"
	"github.com/ChristopherBilg/lazygh/internal/tui/keys"
	"github.com/ChristopherBilg/lazygh/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	os.Exit(run())
}

// run wires up logging and runs the TUI, returning the process exit code. It
// exists so deferred cleanup (closing the log file) runs before exit — os.Exit
// in main would skip defers.
func run() int {
	closeLog, err := logging.Init()
	if err != nil {
		// Pre-TUI: the alt-screen has not started, so a single stderr line is
		// safe and surfaces that diagnostics won't be captured this run.
		fmt.Fprintf(os.Stderr, "lazygh: logging disabled: %v\n", err)
	}
	defer func() { _ = closeLog() }()

	// Load config after logging so config problems are logged, and apply it
	// before the TUI starts. Load never fails; it falls back to defaults.
	cfg := config.Load()
	github.Configure(cfg.GitHub.RESTTimeout, cfg.GitHub.SubprocessTimeout, cfg.GitHub.RepoPageSize)
	styles.Configure(cfg.Theme)
	keys.Configure(cfg.Keys)

	slog.Info("lazygh starting")
	p := tea.NewProgram(tui.NewModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		slog.Error("program exited with error", "err", err)
		fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err) // after TUI closes: safe
		return 1
	}
	slog.Info("lazygh exiting")
	return 0
}
