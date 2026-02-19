package tui

import (
	"fmt"
	"strings"

	"bore-tui/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// HelpModel renders a centered overlay showing all keybindings.
// It is not a screen — it floats on top of whatever screen is active.
type HelpModel struct {
	visible bool
	keys    KeyMap
	styles  theme.Styles
}

// NewHelpModel creates a new help overlay.
func NewHelpModel(keys KeyMap, styles theme.Styles) HelpModel {
	return HelpModel{
		keys:   keys,
		styles: styles,
	}
}

// Toggle flips the overlay visibility.
func (h *HelpModel) Toggle() {
	h.visible = !h.visible
}

// Visible reports whether the overlay is currently showing.
func (h *HelpModel) Visible() bool {
	return h.visible
}

// View renders the help overlay, centered within the given dimensions.
func (h *HelpModel) View(width, height int) string {
	if !h.visible {
		return ""
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorTextPrimary).
		Render("Keybindings")

	bindings := []struct {
		keys string
		desc string
	}{
		{h.keys.Quit.Help().Key, h.keys.Quit.Help().Desc},
		{h.keys.Back.Help().Key, h.keys.Back.Help().Desc},
		{h.keys.Help.Help().Key, h.keys.Help.Help().Desc},
		{h.keys.Tab.Help().Key, h.keys.Tab.Help().Desc},
		{h.keys.Enter.Help().Key, h.keys.Enter.Help().Desc},
		{h.keys.Up.Help().Key, h.keys.Up.Help().Desc},
		{h.keys.Down.Help().Key, h.keys.Down.Help().Desc},
		{h.keys.NewTask.Help().Key, h.keys.NewTask.Help().Desc},
		{h.keys.NewCrew.Help().Key, h.keys.NewCrew.Help().Desc},
		{h.keys.NewThread.Help().Key, h.keys.NewThread.Help().Desc},
		{h.keys.Refresh.Help().Key, h.keys.Refresh.Help().Desc},
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(theme.ColorPrimary).
		Bold(true).
		Width(14)

	descStyle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary)

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	for _, b := range bindings {
		line := keyStyle.Render(b.keys) + descStyle.Render(b.desc)
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Italic(true).
		Render("Press ? to close"))

	content := strings.Join(lines, "\n")
	overlay := h.styles.HelpOverlay.Render(content)

	overlayWidth := lipgloss.Width(overlay)
	overlayHeight := lipgloss.Height(overlay)

	// Center horizontally.
	padLeft := 0
	if width > overlayWidth {
		padLeft = (width - overlayWidth) / 2
	}

	// Center vertically.
	padTop := 0
	if height > overlayHeight {
		padTop = (height - overlayHeight) / 2
	}

	positioned := lipgloss.NewStyle().
		MarginLeft(padLeft).
		MarginTop(padTop).
		Render(overlay)

	// Ensure we fill the exact viewport so the overlay covers underlying content.
	return fmt.Sprintf("%s", positioned)
}
