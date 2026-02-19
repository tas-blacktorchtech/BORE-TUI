package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/theme"
	"bore-tui/internal/web"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// homeAction enumerates the selectable items on the home screen.
// Clusters come first, then the fixed action buttons.
type homeAction int

const (
	homeActionCluster    homeAction = iota // a cluster in the list
	homeActionNewCluster                   // "Create New Cluster"
	homeActionOpenRepo                     // "Open Existing Repo"
	homeActionConfig                       // "Settings"
	homeActionWebGUI                       // "Launch Web GUI"
)

// HomeScreen shows the splash / cluster picker.
type HomeScreen struct {
	app      *app.App
	styles   theme.Styles
	clusters []db.Cluster
	cursor   int
	loaded   bool
	width    int
	height   int
}

// NewHomeScreen creates a new home screen.
func NewHomeScreen(a *app.App, s theme.Styles) HomeScreen {
	return HomeScreen{
		app:    a,
		styles: s,
	}
}

// init returns a command that loads the cluster list from the global state file.
func (s *HomeScreen) init() tea.Cmd {
	a := s.app
	return func() tea.Msg {
		paths := a.KnownClusters()
		var clusters []db.Cluster
		for _, p := range paths {
			clusters = append(clusters, db.Cluster{
				Name:     filepath.Base(p),
				RepoPath: p,
			})
		}
		return ClustersLoadedMsg{Clusters: clusters}
	}
}

// totalItems returns the number of selectable items (clusters + action buttons).
func (s *HomeScreen) totalItems() int {
	return len(s.clusters) + 4 // NewCluster, OpenRepo, Settings, WebGUI
}

// itemAction returns the action type for the item at position i.
func (s *HomeScreen) itemAction(i int) homeAction {
	if i < len(s.clusters) {
		return homeActionCluster
	}
	offset := i - len(s.clusters)
	switch offset {
	case 0:
		return homeActionNewCluster
	case 1:
		return homeActionOpenRepo
	case 2:
		return homeActionConfig
	case 3:
		return homeActionWebGUI
	default:
		return homeActionNewCluster
	}
}

// Update handles messages relevant to the home screen.
func (s *HomeScreen) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ClustersLoadedMsg:
		s.clusters = msg.Clusters
		s.loaded = true
		s.cursor = 0
		return nil

	case tea.KeyMsg:
		km := DefaultKeyMap()

		switch {
		case key.Matches(msg, km.Up):
			if s.cursor > 0 {
				s.cursor--
			}
		case key.Matches(msg, km.Down):
			if s.cursor < s.totalItems()-1 {
				s.cursor++
			}
		case key.Matches(msg, km.Enter):
			return s.selectItem()
		}

	case tea.MouseMsg:
		if !s.loaded || s.totalItems() == 0 {
			return nil
		}

		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if s.cursor > 0 {
				s.cursor--
			}
		case tea.MouseButtonWheelDown:
			if s.cursor < s.totalItems()-1 {
				s.cursor++
			}
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return nil
			}
			// Compute the vertical padding applied by wrapContent.
			// Header: ASCII logo (7 lines), "", subtitle, "" = 10 lines
			// "Recent Clusters" header + blank = 2 lines (when clusters exist)
			// Then cluster items, then "" separator, then action buttons.
			headerLines := 10 // ASCII logo (7), blank, subtitle, blank
			contentLines := headerLines
			if len(s.clusters) > 0 {
				contentLines += 2 // "Recent Clusters" + blank
			} else {
				contentLines += 1 // "No clusters found" line
			}
			contentLines += len(s.clusters) // cluster items
			contentLines += 1               // blank separator before buttons
			contentLines += 4               // 4 action buttons

			padTop := 0
			contentHeight := s.height - 1 // status bar takes 1 line
			if contentHeight > contentLines {
				padTop = (contentHeight - contentLines) / 3
			}

			y := msg.Y - padTop

			// Determine which item was clicked.
			var listStart int
			if len(s.clusters) > 0 {
				listStart = headerLines + 2 // after "Recent Clusters" + blank
			} else {
				listStart = headerLines + 1 // after "No clusters found"
			}

			// Check cluster items.
			if len(s.clusters) > 0 && y >= listStart && y < listStart+len(s.clusters) {
				idx := y - listStart
				s.cursor = idx
				return s.selectItem()
			}

			// Action buttons start after clusters + blank separator.
			buttonsStart := listStart + len(s.clusters) + 1
			if y >= buttonsStart && y < buttonsStart+4 {
				idx := len(s.clusters) + (y - buttonsStart)
				if idx < s.totalItems() {
					s.cursor = idx
					return s.selectItem()
				}
			}
		}
	}
	return nil
}

// selectItem executes the action for the currently selected item.
func (s *HomeScreen) selectItem() tea.Cmd {
	if s.totalItems() == 0 {
		return nil
	}

	action := s.itemAction(s.cursor)
	switch action {
	case homeActionCluster:
		cluster := s.clusters[s.cursor]
		a := s.app
		return func() tea.Msg {
			if err := a.OpenCluster(context.Background(), cluster.RepoPath); err != nil {
				return ErrorMsg{Err: fmt.Errorf("open cluster: %w", err)}
			}
			return ClusterOpenedMsg{}
		}

	case homeActionNewCluster:
		return func() tea.Msg {
			return NavigateMsg{Screen: ScreenCreateCluster}
		}

	case homeActionOpenRepo:
		// "Open Existing Repo" also routes to the create cluster screen
		// with mode set to existing path.
		return func() tea.Msg {
			return NavigateMsg{Screen: ScreenCreateCluster}
		}

	case homeActionConfig:
		return func() tea.Msg {
			return NavigateMsg{Screen: ScreenConfigEditor}
		}

	case homeActionWebGUI:
		a := s.app
		return func() tea.Msg {
			srv := web.New(a)
			url, err := srv.Start(context.Background())
			if err != nil {
				return WebServerErrorMsg{Err: fmt.Errorf("launch web gui: %w", err)}
			}
			return WebServerStartedMsg{URL: url}
		}
	}
	return nil
}

// View renders the home screen.
func (s *HomeScreen) View(width, height int) string {
	s.width = width
	s.height = height
	var sections []string

	// Title / header — ASCII art logo.
	logo := "########   #######  ########  ########\n" +
		"##     ## ##     ## ##     ## ##       \n" +
		"##     ## ##     ## ##     ## ##       \n" +
		"########  ##     ## ########  ######   \n" +
		"##     ## ##     ## ##   ##   ##       \n" +
		"##     ## ##     ## ##    ##  ##       \n" +
		"########   #######  ##     ## ########"

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorTextPrimary).
		Render(logo)

	subtitle := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Render("Agent orchestration for your codebase")

	header := lipgloss.JoinVertical(lipgloss.Left, title, "", subtitle, "")
	sections = append(sections, header)

	// Loading state.
	if !s.loaded {
		loading := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Italic(true).
			Render("  Loading clusters...")
		sections = append(sections, loading)
		return s.wrapContent(sections, width, height)
	}

	// Cluster list.
	if len(s.clusters) > 0 {
		listHeader := lipgloss.NewStyle().
			Foreground(theme.ColorTextPrimary).
			Bold(true).
			Render("  Recent Clusters")
		sections = append(sections, listHeader, "")

		for i, c := range s.clusters {
			label := c.Name
			if c.RepoPath != "" {
				label = fmt.Sprintf("%s  %s",
					c.Name,
					lipgloss.NewStyle().Foreground(theme.ColorTextSecondary).Render(c.RepoPath),
				)
			}
			if i == s.cursor {
				line := s.styles.ListItemSelected.Render(fmt.Sprintf(" > %s", label))
				sections = append(sections, line)
			} else {
				line := s.styles.ListItem.Render(fmt.Sprintf("   %s", label))
				sections = append(sections, line)
			}
		}
	} else {
		empty := lipgloss.NewStyle().
			Foreground(theme.ColorTextSecondary).
			Italic(true).
			PaddingLeft(2).
			Render("No clusters found. Create one to get started.")
		sections = append(sections, empty)
	}

	sections = append(sections, "")

	// Action buttons.
	actions := []string{"Create New Cluster", "Open Existing Repo", "Settings", "Launch Web GUI"}
	for i, label := range actions {
		idx := len(s.clusters) + i
		if idx == s.cursor {
			btn := s.styles.ButtonFocused.Render(label)
			sections = append(sections, "  "+btn)
		} else {
			btn := s.styles.Button.Render(label)
			sections = append(sections, "  "+btn)
		}
	}

	return s.wrapContent(sections, width, height)
}

// wrapContent centers the section content vertically in the available space.
func (s *HomeScreen) wrapContent(sections []string, width, height int) string {
	content := strings.Join(sections, "\n")
	contentHeight := lipgloss.Height(content)

	// Vertically center if we have room.
	padTop := 0
	if height > contentHeight {
		padTop = (height - contentHeight) / 3 // bias toward upper third
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		PaddingTop(padTop).
		PaddingLeft(4).
		Render(content)
}
