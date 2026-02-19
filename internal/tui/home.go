package tui

import (
	"context"
	"fmt"
	"strings"

	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/theme"

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
)

// HomeScreen shows the splash / cluster picker.
type HomeScreen struct {
	app      *app.App
	styles   theme.Styles
	clusters []db.Cluster
	cursor   int
	loaded   bool
}

// NewHomeScreen creates a new home screen.
func NewHomeScreen(a *app.App, s theme.Styles) HomeScreen {
	return HomeScreen{
		app:    a,
		styles: s,
	}
}

// init returns a command that loads the cluster list from the database.
func (s *HomeScreen) init() tea.Cmd {
	a := s.app
	return func() tea.Msg {
		if a.DB() == nil {
			// No database yet — return an empty list so the home screen
			// renders correctly with "No clusters found".
			return ClustersLoadedMsg{Clusters: nil}
		}
		clusters, err := a.ListRecentClusters(context.Background())
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("load clusters: %w", err)}
		}
		return ClustersLoadedMsg{Clusters: clusters}
	}
}

// totalItems returns the number of selectable items (clusters + action buttons).
func (s *HomeScreen) totalItems() int {
	return len(s.clusters) + 3 // NewCluster, OpenRepo, Settings
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
	}
	return nil
}

// View renders the home screen.
func (s *HomeScreen) View(width, height int) string {
	var sections []string

	// Title / header.
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorTextPrimary).
		Background(theme.ColorPrimary).
		Padding(0, 3).
		Render("BORE-TUI")

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
	actions := []string{"Create New Cluster", "Open Existing Repo", "Settings"}
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
