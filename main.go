// mc is a minimal two-pane terminal file navigator inspired by Midnight Commander.
//
// Usage:
//
//	go run .        (run directly from source)
//	go build -o mc  (compile to a binary)
//
// Source layout:
//
//	main.go   — entry point; starts the Bubble Tea event loop
//	model.go  — application state and Bubble Tea lifecycle (Init / Update / View)
//	pane.go   — file-panel struct with all navigation and rendering logic
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// main is the program entry point. It creates a Bubble Tea program and runs
// it in "alt screen" mode, which uses a separate terminal buffer so the
// normal shell output is not overwritten — the same technique used by vim,
// less, and htop.
func main() {
	prog := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running mc: %v\n", err)
		os.Exit(1)
	}
}
