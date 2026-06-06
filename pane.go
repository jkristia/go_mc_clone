package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Lipgloss styles used when rendering a pane.
// These are package-level variables (think module-level constants in Python or
// static readonly fields in C#). They are initialised once at startup.
var (
	activeBorderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39"))
	inactiveBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	selectedStyle       = lipgloss.NewStyle().Background(lipgloss.Color("25")).Foreground(lipgloss.Color("255")).Bold(true)
	dirStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	parentStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	pathStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	colHeaderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// fileEntry represents a single item in a directory listing.
// Think of it as a plain data class / record type — no methods, just fields.
// isParent is true only for the synthetic ".." entry added at the top of every listing.
type fileEntry struct {
	name     string
	isDir    bool
	isParent bool
	size     int64     // bytes; 0 for directories
	mtime    time.Time // last-modified timestamp
}

// pane holds the state of one file-browser panel.
// In C# terms this would be a class; in Python, a dataclass with methods.
// Go uses structs with methods attached via "receivers" (see below).
//
// cursor is the index of the highlighted entry.
// offset is the index of the first visible entry (scroll position).
type pane struct {
	path    string
	entries []fileEntry
	cursor  int
	offset  int
}

// load reads the directory at path, sorts its contents (dirs first, then
// alphabetical), prepends a ".." entry when not at the filesystem root,
// and stores the result in the pane.
//
// The (p *pane) prefix is a "pointer receiver" — equivalent to a method on a
// class in C# or Python. The * means we receive a pointer to the pane so that
// any changes we make are visible to the caller (like ref/out in C# or
// mutating self in Python).
//
// Returns a non-nil error if the path cannot be read (permission denied, etc.).
func (p *pane) load(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return err
	}

	// Sort: directories before files, then case-insensitive alphabetical within each group.
	sort.Slice(des, func(i, j int) bool {
		iDir := des[i].IsDir()
		jDir := des[j].IsDir()
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(des[i].Name()) < strings.ToLower(des[j].Name())
	})

	entries := []fileEntry{}

	// Add a synthetic ".." entry unless we are already at the root of the filesystem.
	if parent := filepath.Dir(abs); parent != abs {
		entries = append(entries, fileEntry{name: "..", isDir: true, isParent: true})
	}

	for _, de := range des {
		fe := fileEntry{name: de.Name(), isDir: de.IsDir()}
		// Info() can fail for broken symlinks or race conditions; we tolerate
		// the failure by leaving size/mtime at their zero values.
		if info, err := de.Info(); err == nil {
			fe.size = info.Size()
			fe.mtime = info.ModTime()
		}
		entries = append(entries, fe)
	}

	p.path = abs
	p.entries = entries
	return nil
}

// selected returns a pointer to the currently highlighted fileEntry, or nil
// if the pane is empty.
//
// Returning a pointer (*fileEntry) lets the caller read the struct without
// copying it. The caller must not store this pointer — it may be invalidated
// the next time load() is called and replaces the entries slice.
func (p *pane) selected() *fileEntry {
	if len(p.entries) == 0 {
		return nil
	}
	return &p.entries[p.cursor]
}

// moveUp moves the cursor one row up, scrolling the viewport if needed.
// visibleRows is passed in so the pane does not need to know about terminal size.
func (p *pane) moveUp(visibleRows int) {
	if p.cursor > 0 {
		p.cursor--
		if p.cursor < p.offset {
			p.offset = p.cursor
		}
	}
}

// moveDown moves the cursor one row down, scrolling the viewport if needed.
func (p *pane) moveDown(visibleRows int) {
	if p.cursor < len(p.entries)-1 {
		p.cursor++
		if p.cursor >= p.offset+visibleRows {
			p.offset = p.cursor - visibleRows + 1
		}
	}
}

// pageUp moves the cursor up by a full page (visibleRows rows).
func (p *pane) pageUp(visibleRows int) {
	p.cursor -= visibleRows
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
}

// pageDown moves the cursor down by a full page (visibleRows rows).
func (p *pane) pageDown(visibleRows int) {
	p.cursor += visibleRows
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
	if p.cursor >= p.offset+visibleRows {
		p.offset = p.cursor - visibleRows + 1
	}
}

// home jumps the cursor to the first entry in the list.
func (p *pane) home() {
	p.cursor = 0
	p.offset = 0
}

// end jumps the cursor to the last entry in the list.
func (p *pane) end(visibleRows int) {
	p.cursor = len(p.entries) - 1
	if p.cursor >= visibleRows {
		p.offset = p.cursor - visibleRows + 1
	}
}

// seekByName positions the cursor on the first entry whose name matches name.
// This is used after navigating into a parent directory so that the cursor
// lands on the subdirectory we just came from, matching Midnight Commander's
// behaviour. If no match is found the cursor stays at the top.
func (p *pane) seekByName(name string, visibleRows int) {
	p.cursor = 0
	p.offset = 0
	for i, e := range p.entries {
		if e.name == name {
			p.cursor = i
			if p.cursor >= visibleRows {
				p.offset = p.cursor - visibleRows + 1
			}
			return
		}
	}
}

// formatSize converts a byte count to a human-readable string of exactly 9
// characters, right-aligned. Directories show "<DIR>" instead of a size.
// The fixed width ensures every row in the file list lines up perfectly.
//
// This is a package-level function (not a method) because it does not need
// any pane state — equivalent to a static helper method in C# or a module-
// level function in Python.
func formatSize(size int64, isDir bool) string {
	if isDir {
		return "    <DIR>"
	}
	switch {
	case size < 1024:
		return fmt.Sprintf("%8d", size)
	case size < 1024*1024:
		return fmt.Sprintf("%6.1f KB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%6.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%6.1f GB", float64(size)/(1024*1024*1024))
	}
}

// render draws the pane into a bordered box and returns it as a string
// containing ANSI escape codes for colours and borders.
//
// Parameters:
//   - isActive:    whether this pane has keyboard focus (affects border colour)
//   - outerWidth:  total column width including the border characters
//   - visibleRows: number of file-list rows to display (excluding path/header lines)
//
// The returned string is assembled by the model's View() and printed to the
// terminal by Bubble Tea. Think of it as the equivalent of a component's
// render() in React / a control's OnPaint in WinForms.
func (p *pane) render(isActive bool, outerWidth, visibleRows int) string {
	// outerWidth includes the two border characters (left and right), so the
	// usable inner width is two less.
	innerWidth := outerWidth - 2
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Truncate the path string to fit, prepending "…" to indicate truncation.
	// We work with []rune (Unicode code points) rather than []byte so that
	// multi-byte characters (accents, CJK, etc.) count as one column each.
	pathStr := p.path
	if runes := []rune(pathStr); len(runes) > innerWidth-1 {
		pathStr = "…" + string(runes[len(runes)-(innerWidth-2):])
	}
	header := pathStyle.Render(pathStr)

	// Column layout:  name (variable) + "  " (2) + size (9) + "  " (2) + date (12) = name + 25
	// So: nameColW = innerWidth - 25
	nameColW := innerWidth - 25
	if nameColW < 8 {
		nameColW = 8
	}
	colHeader := colHeaderStyle.Render(
		fmt.Sprintf("%-*s  %-9s  %-12s", nameColW, "Name", "Size", "Modified"),
	)

	// rows collects every line of content that goes inside the border.
	// In Go, := on a slice literal creates and initialises it in one step.
	rows := []string{header, colHeader}

	end := p.offset + visibleRows
	if end > len(p.entries) {
		end = len(p.entries)
	}

	for i := p.offset; i < end; i++ {
		e := p.entries[i]

		// Append "/" to directory names and truncate with "~" if too long.
		displayName := e.name
		if e.isDir {
			displayName += "/"
		}
		if runes := []rune(displayName); len(runes) > nameColW {
			displayName = string(runes[:nameColW-1]) + "~"
		}

		sizeStr, timeStr := "", ""
		if e.isParent {
			sizeStr = "    <UP> "
		} else {
			sizeStr = formatSize(e.size, e.isDir)
			timeStr = e.mtime.Format("Jan 02 15:04")
		}

		line := fmt.Sprintf("%-*s  %9s  %-12s", nameColW, displayName, sizeStr, timeStr)

		// Apply colour/style based on entry type.
		// "switch" with no condition is Go's clean alternative to if/else-if chains.
		switch {
		case i == p.cursor:
			line = selectedStyle.Width(innerWidth).Render(line)
		case e.isParent:
			line = parentStyle.Render(line)
		case e.isDir:
			line = dirStyle.Render(line)
		}
		rows = append(rows, line)
	}

	// Pad with empty lines so the border always has a fixed height, even when
	// the directory has fewer entries than visibleRows.
	for len(rows) < 2+visibleRows {
		rows = append(rows, "")
	}

	border := inactiveBorderStyle.Width(innerWidth)
	if isActive {
		border = activeBorderStyle.Width(innerWidth)
	}
	return border.Render(strings.Join(rows, "\n"))
}
