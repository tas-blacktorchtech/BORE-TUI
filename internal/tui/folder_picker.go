package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bore-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// FolderSelectedMsg is sent when the user confirms a folder selection.
type FolderSelectedMsg struct{ Path string }

// FolderPickerCancelMsg is sent when the user cancels the folder picker.
type FolderPickerCancelMsg struct{}

// folderEntriesMsg is an internal message carrying directory read results.
type folderEntriesMsg struct {
	dir     string
	entries []string
	err     error
}

// ---------------------------------------------------------------------------
// FolderPicker
// ---------------------------------------------------------------------------

// FolderPicker is an embeddable Bubble Tea sub-model that lets the user
// browse the filesystem and select a directory.
type FolderPicker struct {
	styles theme.Styles

	dir        string   // current directory path
	entries    []string // visible directory entries (dirs only, sorted)
	cursor     int      // index into entries (0 = "..")
	offset     int      // scroll offset for windowed display
	showHidden bool     // whether to show dot-directories
	err        error    // last read error, displayed in the view
}

// NewFolderPicker creates a new folder picker using the given styles.
// The picker starts in the user's home directory.
func NewFolderPicker(styles theme.Styles) FolderPicker {
	return FolderPicker{
		styles: styles,
	}
}

// SetDirectory sets the picker's current directory and returns a command
// that reads the directory entries asynchronously.
func (fp *FolderPicker) SetDirectory(path string) tea.Cmd {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "/"
		}
		path = home
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	fp.dir = abs
	fp.cursor = 0
	fp.offset = 0
	fp.err = nil
	return fp.readDir()
}

// Selected returns the currently displayed directory path, or "" if empty.
func (fp *FolderPicker) Selected() string {
	return fp.dir
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update processes messages for the folder picker and returns any commands.
func (fp *FolderPicker) Update(msg tea.Msg) (*FolderPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case folderEntriesMsg:
		if msg.err != nil {
			fp.err = msg.err
			return fp, nil
		}
		fp.dir = msg.dir
		fp.entries = msg.entries
		fp.cursor = 0
		fp.offset = 0
		fp.err = nil
		return fp, nil

	case tea.KeyMsg:
		return fp.handleKey(msg)

	case tea.MouseMsg:
		return fp.handleMouse(msg)
	}
	return fp, nil
}

// handleKey processes keyboard input for the folder picker.
func (fp *FolderPicker) handleKey(msg tea.KeyMsg) (*FolderPicker, tea.Cmd) {
	total := fp.totalEntries()

	switch msg.String() {
	case "up", "k":
		if total > 0 {
			fp.cursor--
			if fp.cursor < 0 {
				fp.cursor = total - 1
			}
		}
	case "down", "j":
		if total > 0 {
			fp.cursor++
			if fp.cursor >= total {
				fp.cursor = 0
			}
		}
	case "enter", "right", "l":
		return fp.openSelected()
	case "backspace", "left", "h":
		return fp.goUp()
	case ".":
		fp.showHidden = !fp.showHidden
		return fp, fp.readDir()
	case " ":
		// Space confirms the current directory as the selection.
		return fp, func() tea.Msg {
			return FolderSelectedMsg{Path: fp.dir}
		}
	case "esc":
		return fp, func() tea.Msg {
			return FolderPickerCancelMsg{}
		}
	}
	return fp, nil
}

// handleMouse processes mouse input for the folder picker.
func (fp *FolderPicker) handleMouse(msg tea.MouseMsg) (*FolderPicker, tea.Cmd) {
	total := fp.totalEntries()
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if total > 0 {
			fp.cursor--
			if fp.cursor < 0 {
				fp.cursor = total - 1
			}
		}
	case tea.MouseButtonWheelDown:
		if total > 0 {
			fp.cursor++
			if fp.cursor >= total {
				fp.cursor = 0
			}
		}
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return fp, nil
		}
		// Clicks on directory entries: Y offset 2 accounts for the path bar
		// and the blank line. Entries start at line index 2 in the view.
		clickIdx := msg.Y - 2 + fp.offset
		if clickIdx >= 0 && clickIdx < total {
			fp.cursor = clickIdx
			return fp.openSelected()
		}
	}
	return fp, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the folder picker into the given width and height.
func (fp *FolderPicker) View(width, height int) string {
	if width < 10 || height < 4 {
		return ""
	}

	var b strings.Builder

	// Path bar.
	pathStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Background(theme.ColorPrimary).
		Bold(true).
		Padding(0, 1).
		MaxWidth(width)
	b.WriteString(pathStyle.Render(fp.dir))
	b.WriteString("\n")

	// Error display.
	if fp.err != nil {
		errStyle := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true)
		b.WriteString(errStyle.Render(fmt.Sprintf("  Error: %s", fp.err.Error())))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	// Available lines for directory entries (path bar + blank/error = 2 lines,
	// hint line at the bottom = 1 line).
	listHeight := height - 3
	if listHeight < 1 {
		listHeight = 1
	}

	total := fp.totalEntries()

	// Adjust scroll offset to keep cursor visible.
	if fp.cursor < fp.offset {
		fp.offset = fp.cursor
	}
	if fp.cursor >= fp.offset+listHeight {
		fp.offset = fp.cursor - listHeight + 1
	}

	// Render visible entries.
	normalStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		PaddingLeft(1)
	selectedStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextPrimary).
		Background(theme.ColorPrimary).
		Bold(true).
		PaddingLeft(1)
	dimStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		PaddingLeft(1)

	linesRendered := 0
	for i := fp.offset; i < total && linesRendered < listHeight; i++ {
		name := fp.entryName(i)
		isHidden := len(name) > 0 && name[0] == '.' && name != ".."

		var line string
		if i == fp.cursor {
			line = selectedStyle.Width(width).Render("> " + name)
		} else if isHidden {
			line = dimStyle.Width(width).Render("  " + name)
		} else {
			line = normalStyle.Width(width).Render("  " + name)
		}
		b.WriteString(line)
		linesRendered++
		if linesRendered < listHeight {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines.
	for linesRendered < listHeight {
		b.WriteString("\n")
		linesRendered++
	}

	// Hint bar.
	hintStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Italic(true)
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  Space: select  Enter: open  .: toggle hidden  Esc: cancel"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// totalEntries returns the number of visible entries including the ".." entry.
func (fp *FolderPicker) totalEntries() int {
	return len(fp.entries) + 1 // +1 for ".."
}

// entryName returns the display name for entry at the given index.
// Index 0 is always "..".
func (fp *FolderPicker) entryName(i int) string {
	if i == 0 {
		return ".."
	}
	return fp.entries[i-1]
}

// openSelected navigates into the selected directory or goes up if ".." is selected.
func (fp *FolderPicker) openSelected() (*FolderPicker, tea.Cmd) {
	if fp.totalEntries() == 0 {
		return fp, nil
	}
	name := fp.entryName(fp.cursor)
	if name == ".." {
		return fp.goUp()
	}
	target := filepath.Join(fp.dir, name)
	fp.dir = target
	return fp, fp.readDir()
}

// goUp navigates to the parent directory.
func (fp *FolderPicker) goUp() (*FolderPicker, tea.Cmd) {
	parent := filepath.Dir(fp.dir)
	if parent == fp.dir {
		// Already at root.
		return fp, nil
	}
	fp.dir = parent
	return fp, fp.readDir()
}

// readDir returns a command that reads the current directory and produces
// a folderEntriesMsg.
func (fp *FolderPicker) readDir() tea.Cmd {
	dir := fp.dir
	showHidden := fp.showHidden
	return func() tea.Msg {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return folderEntriesMsg{dir: dir, err: fmt.Errorf("read dir: %w", err)}
		}

		var dirs []string
		for _, e := range dirEntries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if !showHidden && len(name) > 0 && name[0] == '.' {
				continue
			}
			dirs = append(dirs, name)
		}

		sort.Strings(dirs)
		return folderEntriesMsg{dir: dir, entries: dirs}
	}
}
