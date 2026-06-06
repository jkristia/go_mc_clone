# mc — Midnight Commander Clone

A minimal two-pane terminal file navigator written in Go.

## Running and building

```bash
go run .                                          # run from source
go build -o mc .                                  # build for current platform
GOOS=linux   GOARCH=amd64 go build -o mc-linux . # cross-compile
GOOS=darwin  GOARCH=arm64 go build -o mc-mac .
GOOS=windows GOARCH=amd64 go build -o mc.exe .
```

After changing dependencies: `go mod tidy`

## Project layout

```
main.go   — entry point; starts the Bubble Tea event loop (alt-screen mode)
model.go  — application state; Bubble Tea lifecycle: Init / Update / View
pane.go   — pane struct: directory loading, navigation, rendering
go.mod / go.sum — module definition and locked dependency checksums
```

## Key types

| Type | File | Purpose |
|---|---|---|
| `fileEntry` | pane.go | One file/dir row (name, size, mtime, flags) |
| `pane` | pane.go | One panel: path, entries slice, cursor, scroll offset |
| `model` | model.go | Top-level Bubble Tea state: two panes + terminal dimensions |

## Architecture — Bubble Tea (Elm pattern)

Bubble Tea follows the Elm / Redux pattern. Every event goes through one cycle:

```
event → Update(model) → new model → View(model) → terminal string
```

- **`Init`** — returns startup commands (nil here; nothing to do)
- **`Update`** — receives a `tea.Msg` (key press, resize), returns updated model + optional command
- **`View`** — pure function, converts model to an ANSI string; must not mutate state

The model is passed **by value** — changes are made to a local copy and returned. This is intentional (immutability by convention).

## Go idioms used in this project

- **Pointer receiver** `(p *pane)` — method mutates the receiver (like `ref` in C#)
- **Value receiver** `(m model)` — method gets a copy; used on `View` to enforce purity
- **Type switch** `switch msg := msg.(type)` — safe union unwrapping (like TS discriminated unions)
- **`[]rune` for string width** — multi-byte Unicode chars count as one column; never index strings by byte when displaying
- **`filepath.Abs / Dir / Base / Join`** — always use `filepath` (not `path`) for OS-correct separators on Windows

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/charmbracelet/bubbletea` | TUI event loop (the Elm/Redux engine) |
| `github.com/charmbracelet/lipgloss` | Terminal styling: colours, borders, layout |

## Keyboard bindings (defined in `model.go Update`)

| Key | Action |
|---|---|
| `Tab` | Switch active pane |
| `↑↓` / `jk` | Move cursor |
| `PgUp/PgDn` / `Ctrl+U/D` | Scroll a full page |
| `g` / `G` | Jump to top / bottom |
| `Enter` / `→` / `l` | Enter directory |
| `←` / `Backspace` / `h` | Go up to parent |
| `q` / `Ctrl+C` | Quit |

## Layout math (important for rendering bugs)

Each pane renders into `outerWidth` columns:
- `innerWidth = outerWidth - 2` (border left + right)
- Column layout inside: `nameColW + 25` must equal `innerWidth`
  - Fixed part: 2 (gap) + 9 (size) + 2 (gap) + 12 (date) = **25**
  - So: `nameColW = innerWidth - 25`
- Total pane height: `border(2) + path(1) + colheader(1) + visibleRows`
- `visibleRows = terminalHeight - 7`
  - 7 = border top+bottom(2) + path(1) + colheader(1) + statusbar(1) + helpbar(1) + safety margin(1)

## User background

The developer is experienced in TypeScript, Python, and C# but new to Go. Comments
in the source intentionally map Go idioms back to equivalents in those languages.
