package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	previewBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39"))
	previewTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true).Background(lipgloss.Color("24"))
	previewMetaStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(lipgloss.Color("24"))
	scrollTrackStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	scrollThumbStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	lineNumStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// preview holds the state of the file-preview overlay.
type preview struct {
	path   string
	lines  []string
	size   int64
	mtime  time.Time
	offset int
	open   bool
}

// load reads the file at path, captures its metadata, and populates lines.
// Binary files (containing a null byte) show a notice instead of raw content.
func (pv *preview) load(path string) {
	pv.path = path
	pv.offset = 0
	pv.open = true
	pv.lines = nil

	info, err := os.Stat(path)
	if err == nil {
		pv.size = info.Size()
		pv.mtime = info.ModTime()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		pv.lines = []string{fmt.Sprintf("Cannot read file: %v", err)}
		return
	}
	for _, b := range data {
		if b == 0 {
			pv.lines = []string{"[binary file — cannot preview]"}
			return
		}
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	pv.lines = strings.Split(content, "\n")
	// strings.Split on a file ending with \n produces a trailing empty element; drop it.
	if n := len(pv.lines); n > 0 && pv.lines[n-1] == "" {
		pv.lines = pv.lines[:n-1]
	}
}

func (pv *preview) close() {
	pv.open = false
	pv.lines = nil
	pv.offset = 0
	pv.path = ""
}

func (pv *preview) scrollUp(n int) {
	pv.offset -= n
	if pv.offset < 0 {
		pv.offset = 0
	}
}

func (pv *preview) scrollDown(n, visibleLines int) {
	top := len(pv.lines) - visibleLines
	if top < 0 {
		top = 0
	}
	pv.offset += n
	if pv.offset > top {
		pv.offset = top
	}
}

// previewVisibleLines returns how many content rows fit inside the overlay for
// a given terminal height. The overlay box takes termHeight-4 rows (2-row margin
// top and bottom); the border and title row consume 3 more.
func previewVisibleLines(termHeight int) int {
	v := termHeight - 4 - 2 - 1 // margin(4) + border(2) + title(1)
	if v < 1 {
		return 1
	}
	return v
}

// render builds the overlay string centered by lipgloss.Place in model.View().
func (pv *preview) render(termWidth, termHeight int) string {
	margin := 4
	boxWidth := termWidth - margin*2
	if boxWidth < 24 {
		boxWidth = 24
	}
	boxHeight := termHeight - 4
	if boxHeight < 6 {
		boxHeight = 6
	}

	innerWidth := boxWidth - 2
	visibleLines := boxHeight - 2 - 1 // border rows + title row

	// Gutter: right-aligned number + "│" + space = numDigits + 2 chars.
	numDigits := len(fmt.Sprintf("%d", len(pv.lines)))
	lineNumWidth := numDigits + 2
	contentWidth := innerWidth - 1 - lineNumWidth // -1 for scrollbar

	// Clamp scroll offset.
	top := len(pv.lines) - visibleLines
	if top < 0 {
		top = 0
	}
	if pv.offset > top {
		pv.offset = top
	}

	// Title: filename on the left, size + date on the right, both on the same
	// background so they read as one bar.
	filename := filepath.Base(pv.path)
	meta := fmt.Sprintf(" %s  %s ", formatSize(pv.size, false), pv.mtime.Format("Jan 02 2006 15:04"))
	metaW := len([]rune(meta))
	nameW := innerWidth - metaW
	if nameW < 1 {
		nameW = 1
	}
	nameRunes := []rune(" " + filename)
	if len(nameRunes) > nameW {
		nameRunes = nameRunes[:nameW-1]
		nameRunes = append(nameRunes, '…')
	}
	namePart := previewTitleStyle.Width(nameW).Render(string(nameRunes))
	metaPart := previewMetaStyle.Width(metaW).Render(meta)
	titleLine := namePart + metaPart

	// Scrollbar and content rows.
	scrollbar := buildScrollbar(len(pv.lines), pv.offset, visibleLines)

	rows := make([]string, visibleLines)
	for i := range rows {
		idx := pv.offset + i
		var gutter, text string
		if idx < len(pv.lines) {
			gutter = lineNumStyle.Render(fmt.Sprintf("%*d│ ", numDigits, idx+1))
			text = strings.ReplaceAll(pv.lines[idx], "\t", "    ")
		} else {
			gutter = lineNumStyle.Render(strings.Repeat(" ", lineNumWidth))
		}
		runes := []rune(text)
		if len(runes) > contentWidth {
			text = string(runes[:contentWidth])
		} else {
			text = text + strings.Repeat(" ", contentWidth-len(runes))
		}
		rows[i] = gutter + text + scrollbar[i]
	}

	content := strings.Join(append([]string{titleLine}, rows...), "\n")
	return previewBorderStyle.Width(innerWidth).Render(content)
}

// buildScrollbar returns one styled string per visible line representing the
// scroll track and thumb.
func buildScrollbar(totalLines, offset, visibleLines int) []string {
	bar := make([]string, visibleLines)
	for i := range bar {
		bar[i] = scrollTrackStyle.Render("│")
	}
	if totalLines <= visibleLines {
		return bar
	}
	thumbSize := max(1, visibleLines*visibleLines/totalLines)
	maxOffset := totalLines - visibleLines
	thumbPos := 0
	if maxOffset > 0 {
		thumbPos = (offset * (visibleLines - thumbSize)) / maxOffset
	}
	for i := thumbPos; i < thumbPos+thumbSize && i < visibleLines; i++ {
		bar[i] = scrollThumbStyle.Render("█")
	}
	return bar
}
