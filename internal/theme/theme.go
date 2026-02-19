package theme

import "github.com/charmbracelet/lipgloss"

// Color constants for the bore-tui dark theme.
const (
	ColorBackground    = lipgloss.Color("#0b0f1a")
	ColorPanel         = lipgloss.Color("#11182a")
	ColorPrimary       = lipgloss.Color("#1e3a8a")
	ColorAccent        = lipgloss.Color("#dc2626")
	ColorTextPrimary   = lipgloss.Color("#e5e7eb")
	ColorTextSecondary = lipgloss.Color("#9ca3af")
	ColorBorderSoft    = lipgloss.Color("#24324f")
	ColorDiffAdd       = lipgloss.Color("#22c55e")
	ColorDiffDelete    = lipgloss.Color("#dc2626")
	ColorSuccess       = lipgloss.Color("#22c55e")
	ColorWarning       = lipgloss.Color("#f59e0b")
)

// Styles holds every lipgloss style used across the TUI.
type Styles struct {
	Panel        lipgloss.Style
	PanelFocused lipgloss.Style

	Header lipgloss.Style

	ListItem         lipgloss.Style
	ListItemSelected lipgloss.Style

	Button        lipgloss.Style
	ButtonFocused lipgloss.Style
	ButtonDanger  lipgloss.Style

	BadgeRunning     lipgloss.Style
	BadgeCompleted   lipgloss.Style
	BadgeFailed      lipgloss.Style
	BadgeInterrupted lipgloss.Style

	DiffAddition lipgloss.Style
	DiffDeletion lipgloss.Style
	DiffContext  lipgloss.Style

	LogDebug lipgloss.Style
	LogInfo  lipgloss.Style
	LogWarn  lipgloss.Style
	LogError lipgloss.Style

	Input        lipgloss.Style
	InputFocused lipgloss.Style

	TabActive   lipgloss.Style
	TabInactive lipgloss.Style

	CommandBar  lipgloss.Style
	HelpOverlay lipgloss.Style
	StatusBar   lipgloss.Style
}

// DefaultStyles returns the default set of styles for bore-tui.
// Callers receive a value copy, so mutations stay local.
func DefaultStyles() Styles {
	return Styles{
		Panel: lipgloss.NewStyle().
			Background(ColorPanel).
			Foreground(ColorTextPrimary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderSoft).
			Padding(1, 2),

		PanelFocused: lipgloss.NewStyle().
			Background(ColorPanel).
			Foreground(ColorTextPrimary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2),

		Header: lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(ColorTextPrimary).
			Bold(true).
			Padding(0, 2),

		ListItem: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			PaddingLeft(2),

		ListItemSelected: lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(ColorTextPrimary).
			Bold(true).
			PaddingLeft(2),

		Button: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPanel).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderSoft).
			Padding(0, 3),

		ButtonFocused: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPrimary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Bold(true).
			Padding(0, 3),

		ButtonDanger: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorAccent).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Bold(true).
			Padding(0, 3),

		BadgeRunning: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorPrimary).
			Bold(true).
			Padding(0, 1),

		BadgeCompleted: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorSuccess).
			Bold(true).
			Padding(0, 1),

		BadgeFailed: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorAccent).
			Bold(true).
			Padding(0, 1),

		BadgeInterrupted: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorWarning).
			Bold(true).
			Padding(0, 1),

		DiffAddition: lipgloss.NewStyle().
			Foreground(ColorDiffAdd),

		DiffDeletion: lipgloss.NewStyle().
			Foreground(ColorDiffDelete),

		DiffContext: lipgloss.NewStyle().
			Foreground(ColorTextSecondary),

		LogDebug: lipgloss.NewStyle().
			Foreground(ColorTextSecondary),

		LogInfo: lipgloss.NewStyle().
			Foreground(ColorTextPrimary),

		LogWarn: lipgloss.NewStyle().
			Foreground(ColorWarning),

		LogError: lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true),

		Input: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPanel).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderSoft).
			Padding(0, 1),

		InputFocused: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPanel).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1),

		TabActive: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPrimary).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorAccent),

		TabInactive: lipgloss.NewStyle().
			Foreground(ColorTextSecondary).
			Background(ColorPanel).
			Padding(0, 2).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorBorderSoft),

		CommandBar: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPanel).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(ColorBorderSoft),

		HelpOverlay: lipgloss.NewStyle().
			Foreground(ColorTextPrimary).
			Background(ColorPanel).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 3),

		StatusBar: lipgloss.NewStyle().
			Foreground(ColorTextSecondary).
			Background(ColorBackground).
			Padding(0, 1),
	}
}
