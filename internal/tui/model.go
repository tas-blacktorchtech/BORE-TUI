package tui

import (
	"fmt"

	"bore-tui/internal/app"
	"bore-tui/internal/db"
	"bore-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the central Bubble Tea model that dispatches to screen models.
type Model struct {
	app    *app.App
	styles theme.Styles
	keys   KeyMap
	help   HelpModel

	screen      Screen
	screenStack []Screen
	width       int
	height      int
	err         error
	status      string

	home             HomeScreen
	createCluster    CreateClusterScreen
	dashboard        DashboardScreen
	commanderBuilder CommanderBuilderScreen
	crewManager      CrewManagerScreen
	newTask          NewTaskScreen
	commanderReview  CommanderReviewScreen
	executionView    ExecutionViewScreen
	diffReview       DiffReviewScreen
	configEditor     ConfigEditorScreen
}

// ---------------------------------------------------------------------------
// Model constructor
// ---------------------------------------------------------------------------

// NewModel creates the top-level TUI model with default styles and all screens.
func NewModel(a *app.App) Model {
	styles := theme.DefaultStyles()
	keys := DefaultKeyMap()

	return Model{
		app:    a,
		styles: styles,
		keys:   keys,
		help:   NewHelpModel(keys, styles),
		screen: ScreenHome,

		home:             NewHomeScreen(a, styles),
		createCluster:    NewCreateClusterScreen(a, styles),
		dashboard:        NewDashboardScreen(a, styles),
		commanderBuilder: NewCommanderBuilderScreen(a, styles),
		crewManager:      NewCrewManagerScreen(a, styles),
		newTask:          NewNewTaskScreen(a, styles),
		commanderReview:  NewCommanderReviewScreen(a, styles),
		executionView:    NewExecutionViewScreen(a, styles),
		diffReview:       NewDiffReviewScreen(a, styles),
		configEditor:     NewConfigEditorScreen(a, styles),
	}
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------

// Init returns the initial command: enter alt screen and load the home screen.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.home.init(),
	)
}

// Update handles all incoming messages by routing to the active screen
// and processing global keys and navigation messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward WindowSizeMsg to the active screen so screens that
		// store their own dimensions stay in sync.
		cmd := m.updateActiveScreen(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// Help overlay intercepts ? regardless of screen.
		if msg.String() == "?" {
			m.help.Toggle()
			return m, nil
		}

		// If help is visible, consume all other keys to dismiss.
		if m.help.Visible() {
			m.help.Toggle()
			return m, nil
		}

		// Global quit: ctrl+c always quits.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// q quits only from the home screen to avoid accidental exits.
		if msg.String() == "q" && m.screen == ScreenHome {
			return m, tea.Quit
		}

		// Do NOT intercept esc globally. Individual screens handle esc
		// themselves (dashboard clears filters, builders cancel forms,
		// etc.). Screens that want to navigate back send NavigateBackMsg.

	case NavigateMsg:
		m.screenStack = append(m.screenStack, m.screen)
		m.screen = msg.Screen
		m.err = nil
		m.status = ""
		cmd := m.initScreen(msg.Screen, msg.Data)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case NavigateBackMsg:
		m.err = nil
		m.status = ""
		if len(m.screenStack) > 0 {
			prev := m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
			m.screen = prev
			cmds = append(cmds, m.initScreen(prev, nil))
		} else {
			m.screen = ScreenHome
			cmds = append(cmds, m.home.init())
		}
		return m, tea.Batch(cmds...)

	case ErrorMsg:
		m.err = msg.Err
		return m, nil

	case StatusMsg:
		m.status = string(msg)
		return m, nil

	case ClusterOpenedMsg:
		m.screen = ScreenDashboard
		m.status = "Cluster opened"
		cmds = append(cmds, m.dashboard.Init())
		return m, tea.Batch(cmds...)

	case ClusterInitDoneMsg:
		m.screen = ScreenDashboard
		m.status = "Cluster created"
		cmds = append(cmds, m.dashboard.Init())
		return m, tea.Batch(cmds...)
	}

	// Delegate to the active screen.
	cmd := m.updateActiveScreen(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the active screen with an optional help overlay and status bar.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Reserve 1 line for the status bar.
	contentHeight := m.height - 1

	// Render active screen content.
	content := m.viewActiveScreen(m.width, contentHeight)

	// Build the status bar.
	statusBar := m.renderStatusBar()

	// Combine content and status bar.
	view := lipgloss.JoinVertical(lipgloss.Left, content, statusBar)

	// If help overlay is visible, render it on top.
	if m.help.Visible() {
		return m.help.View(m.width, m.height)
	}

	return view
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// initScreen sends the appropriate initialization command when navigating
// to a new screen.
func (m *Model) initScreen(screen Screen, data any) tea.Cmd {
	switch screen {
	case ScreenHome:
		return m.home.init()
	case ScreenCreateCluster:
		m.createCluster.reset()
		return nil
	case ScreenDashboard:
		return m.dashboard.Init()
	case ScreenCommanderBuilder:
		return m.commanderBuilder.Init()
	case ScreenCrewManager:
		return m.crewManager.Init()
	case ScreenNewTask:
		return m.newTask.Init()
	case ScreenCommanderReview:
		// If data contains a *db.Task, initialize the review with it.
		if task, ok := data.(*db.Task); ok {
			return m.commanderReview.SetTask(task)
		}
		return m.commanderReview.Init()
	case ScreenExecutionView:
		// If data contains an execStartData, initialize with brief and task.
		if esd, ok := data.(execStartData); ok {
			return m.executionView.SetExecutionWithBrief(esd.Execution, esd.Brief, esd.Task)
		}
		// If data contains a *db.Execution, initialize the view with it.
		if exec, ok := data.(*db.Execution); ok {
			return m.executionView.SetExecution(exec)
		}
		return m.executionView.Init()
	case ScreenDiffReview:
		// If data contains a *db.Execution, initialize the review with it.
		if exec, ok := data.(*db.Execution); ok {
			return m.diffReview.SetExecution(exec)
		}
		return m.diffReview.Init()
	case ScreenConfigEditor:
		m.configEditor.loadFields()
		return nil
	default:
		return nil
	}
}

// updateActiveScreen delegates Update to whichever screen is active.
// Screens from other agents use value-receiver Update returning
// (ScreenType, tea.Cmd). Screens defined here (home, createCluster,
// configEditor) use pointer-receiver Update returning tea.Cmd.
func (m *Model) updateActiveScreen(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd

	switch m.screen {
	case ScreenHome:
		return m.home.Update(msg)
	case ScreenCreateCluster:
		return m.createCluster.Update(msg)
	case ScreenDashboard:
		return m.dashboard.Update(msg)
	case ScreenCommanderBuilder:
		return m.commanderBuilder.Update(msg)
	case ScreenCrewManager:
		return m.crewManager.Update(msg)
	case ScreenNewTask:
		return m.newTask.Update(msg)
	case ScreenCommanderReview:
		m.commanderReview, cmd = m.commanderReview.Update(msg)
		return cmd
	case ScreenExecutionView:
		m.executionView, cmd = m.executionView.Update(msg)
		return cmd
	case ScreenDiffReview:
		m.diffReview, cmd = m.diffReview.Update(msg)
		return cmd
	case ScreenConfigEditor:
		return m.configEditor.Update(msg)
	default:
		return nil
	}
}

// viewActiveScreen delegates View to whichever screen is active.
// Screens from other agents use View() with no args (they store dimensions
// internally via tea.WindowSizeMsg). Screens defined here (home,
// createCluster, configEditor) accept (width, height).
func (m *Model) viewActiveScreen(width, height int) string {
	switch m.screen {
	case ScreenHome:
		return m.home.View(width, height)
	case ScreenCreateCluster:
		return m.createCluster.View(width, height)
	case ScreenDashboard:
		return m.dashboard.View(width, height)
	case ScreenCommanderBuilder:
		return m.commanderBuilder.View(width, height)
	case ScreenCrewManager:
		return m.crewManager.View(width, height)
	case ScreenNewTask:
		return m.newTask.View(width, height)
	case ScreenCommanderReview:
		return m.commanderReview.View()
	case ScreenExecutionView:
		return m.executionView.View()
	case ScreenDiffReview:
		return m.diffReview.View()
	case ScreenConfigEditor:
		return m.configEditor.View(width, height)
	default:
		return ""
	}
}

// renderStatusBar builds the single-line bar at the bottom of the viewport.
func (m *Model) renderStatusBar() string {
	var left string
	if m.err != nil {
		left = lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true).
			Render(fmt.Sprintf(" Error: %s", m.err.Error()))
	} else if m.status != "" {
		left = lipgloss.NewStyle().
			Foreground(theme.ColorSuccess).
			Render(fmt.Sprintf(" %s", m.status))
	}

	right := lipgloss.NewStyle().
		Foreground(theme.ColorTextSecondary).
		Render("? help ")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	spacer := lipgloss.NewStyle().Width(gap).Render("")

	bar := lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, right)

	return m.styles.StatusBar.Width(m.width).Render(bar)
}
