package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Pane index constants used as the model.active field value.
// In Go, const blocks are like enums — plain integers with named aliases.
const (
	leftPane  = 0
	rightPane = 1
)

// Styles used by the model layer (status bar, help line, error text).
// Defined here rather than in pane.go because only View() and statusMessage() use them.
var (
	statusBarStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252")).Padding(0, 1)
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// helpText is the one-line keyboard reference shown at the bottom of the screen.
// It is a package-level constant because it never changes at runtime.
const helpText = " Tab:switch pane  ↑↓/jk:navigate  PgUp/PgDn:scroll  Enter/→:open  ←/Bksp:up  g/G:top/end  q:quit"

// model is the top-level application state managed by Bubble Tea.
//
// Bubble Tea follows the Elm architecture (also used by Redux in JS):
//   - model  holds all application state (like a Redux store)
//   - Init   returns any startup commands
//   - Update receives an event and returns a new model  (like a reducer)
//   - View   converts the model to a string for the terminal (like a render function)
//
// Note that model is passed by VALUE to Update and View, so each call gets a
// snapshot. To mutate state you change the local copy and return it. This is
// unlike a class in C# or Python where methods mutate the instance in place.
type model struct {
	left   pane
	right  pane
	active int // leftPane or rightPane
	width  int // current terminal width in columns
	height int // current terminal height in rows
	err    error
}

// initialModel constructs the starting state of the application.
// Both panes begin in the current working directory.
// In C# this would be a constructor; in Python, __init__.
func initialModel() model {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	m := model{active: leftPane}
	if err := m.left.load(cwd); err != nil {
		m.err = err
	}
	if err := m.right.load(cwd); err != nil {
		m.err = err
	}
	return m
}

// currentPane returns a pointer to whichever pane currently has focus.
// The * receiver means we are working on the actual model, not a copy — any
// changes through the returned pointer will be visible to the caller.
func (m *model) currentPane() *pane {
	if m.active == leftPane {
		return &m.left
	}
	return &m.right
}

// visibleRows calculates how many file-list rows fit inside a pane given the
// current terminal height. The magic number 7 accounts for:
//
//	border top (1) + path header (1) + column header (1) + border bottom (1)
//	+ status bar (1) + help bar (1) + 1 safety margin to prevent overflow
func (m *model) visibleRows() int {
	rows := m.height - 7
	if rows < 1 {
		return 1
	}
	return rows
}

// Init is called once by Bubble Tea when the program starts.
// It can return a tea.Cmd to kick off background work (HTTP requests, timers,
// etc.). We return nil because this app needs no startup commands.
//
// This satisfies the tea.Model interface — the Go equivalent of implementing
// an interface in C# or a protocol/ABC in Python. All three methods (Init,
// Update, View) must be present for the type to satisfy tea.Model.
func (m model) Init() tea.Cmd {
	return nil
}

// Update is called by Bubble Tea every time an event arrives (key press,
// window resize, etc.). It receives the current model as a value copy,
// applies any changes, and returns the new model plus an optional command.
//
// The "msg tea.Msg" parameter is an interface{} / any — a type-switched union.
// The switch msg := msg.(type) pattern is Go's way of safely unwrapping it,
// similar to pattern matching in modern C# or TypeScript's discriminated unions.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// WindowSizeMsg fires once at startup and again whenever the terminal is resized.
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.err = nil
		vr := m.visibleRows()
		p := m.currentPane()

		// msg.String() returns a normalised key name such as "up", "ctrl+c", "tab".
		switch msg.String() {
		case "q", "ctrl+c":
			// tea.Quit is a special command that tells Bubble Tea to stop the loop.
			return m, tea.Quit

		case "tab":
			// Flip between 0 and 1 with integer arithmetic — no if/else needed.
			m.active = 1 - m.active

		case "up", "k":
			p.moveUp(vr)

		case "down", "j":
			p.moveDown(vr)

		case "pgup", "ctrl+u":
			p.pageUp(vr)

		case "pgdown", "ctrl+d":
			p.pageDown(vr)

		case "home", "g":
			p.home()

		case "end", "G":
			p.end(vr)

		case "enter", "right", "l":
			sel := p.selected()
			if sel != nil && sel.isDir {
				var newPath string
				if sel.isParent {
					newPath = filepath.Dir(p.path)
				} else {
					newPath = filepath.Join(p.path, sel.name)
				}
				prevPath := p.path
				if err := p.load(newPath); err != nil {
					m.err = err
				} else if sel.isParent {
					// After going up, restore the cursor to the subdirectory we
					// just left so the user can see where they came from.
					p.seekByName(filepath.Base(prevPath), vr)
				}
			}

		case "backspace", "left", "h":
			parent := filepath.Dir(p.path)
			if parent != p.path {
				// Capture the current dir name before loading the parent so
				// we can seek the cursor back to it afterward.
				prevName := filepath.Base(p.path)
				if err := p.load(parent); err != nil {
					m.err = err
				} else {
					p.seekByName(prevName, vr)
				}
			}
		}
	}
	return m, nil
}

// View converts the current model into a string of ANSI-escaped terminal output.
// Bubble Tea calls this after every Update and writes the result to the screen.
// It must be a pure function — it must not modify the model.
//
// Note the value receiver (m model) rather than a pointer receiver (*model):
// this makes it impossible to accidentally mutate the model inside View, which
// mirrors the constraint that render functions in React/Elm must be pure.
func (m model) View() string {
	if m.width == 0 {
		// The first WindowSizeMsg has not arrived yet; show a placeholder.
		return "Initializing…"
	}

	vr := m.visibleRows()

	// Split the terminal width evenly between the two panes.
	// Integer division truncates, so the right pane gets the remainder column
	// when the total width is odd.
	paneWidth := m.width / 2
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.left.render(m.active == leftPane, paneWidth, vr),
		m.right.render(m.active == rightPane, m.width-paneWidth, vr),
	)

	// Truncate the status message to prevent it from wrapping and adding an
	// extra line. We subtract 2 because statusBarStyle has Padding(0, 1)
	// which adds one space on each side.
	statusMsg := m.statusMessage()
	if runes := []rune(statusMsg); len(runes) > m.width-2 && m.width > 2 {
		statusMsg = string(runes[:m.width-2])
	}
	statusBar := statusBarStyle.Width(m.width - 2).Render(statusMsg)

	// Truncate the help line to terminal width for the same reason.
	helpRunes := []rune(helpText)
	if len(helpRunes) > m.width {
		helpRunes = helpRunes[:m.width]
	}
	help := helpStyle.Render(string(helpRunes))

	// JoinVertical stacks strings top-to-bottom, inserting "\n" between them.
	return lipgloss.JoinVertical(lipgloss.Left, panes, statusBar, help)
}

// statusMessage builds the one-line description shown in the status bar.
// It is split out from View() to keep View focused on layout.
func (m model) statusMessage() string {
	if m.err != nil {
		return errorStyle.Render("Error: " + m.err.Error())
	}
	p := m.currentPane()
	sel := p.selected()
	if sel == nil {
		return ""
	}
	switch {
	case sel.isParent:
		return fmt.Sprintf("Go up to: %s", filepath.Dir(p.path))
	case sel.isDir:
		return fmt.Sprintf("Dir:  %s", filepath.Join(p.path, sel.name))
	default:
		return fmt.Sprintf("File: %s  (%s)", sel.name, formatSize(sel.size, false))
	}
}
