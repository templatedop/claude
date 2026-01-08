package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents the current view in the TUI.
type ViewMode int

const (
	ViewMain ViewMode = iota
	ViewWorkflows
	ViewTasks
	ViewConfig
	ViewHelp
)

// Task represents a task in the todo list.
type Task struct {
	ID          string
	Title       string
	Description string
	Status      string // pending, in_progress, completed, failed
	AgentType   string
	StartTime   time.Time
	EndTime     time.Time
	Error       string
}

// Workflow represents a workflow execution.
type Workflow struct {
	ID         string
	Title      string
	Status     string
	Tasks      []Task
	StartTime  time.Time
	EndTime    time.Time
	TokensUsed int
}

// Model is the main TUI model.
type Model struct {
	// Current view
	view ViewMode

	// UI components
	spinner   spinner.Model
	textInput textinput.Model

	// Data
	workflows       []Workflow
	selectedIdx     int
	tasks           []Task
	taskSelectedIdx int

	// State
	loading   bool
	err       error
	width     int
	height    int
	quitting  bool

	// Config display
	configKeys   []string
	configValues map[string]string
}

// NewModel creates a new TUI model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	ti := textinput.New()
	ti.Placeholder = "Enter task description..."
	ti.CharLimit = 256
	ti.Width = 50

	return Model{
		view:         ViewMain,
		spinner:      s,
		textInput:    ti,
		workflows:    []Workflow{},
		tasks:        []Task{},
		configValues: make(map[string]string),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Message types
type (
	workflowsLoadedMsg struct {
		workflows []Workflow
	}
	tasksLoadedMsg struct {
		tasks []Task
	}
	errMsg struct {
		err error
	}
	tickMsg time.Time
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "tab":
			// Cycle through views
			m.view = (m.view + 1) % 5
			return m, nil

		case "1":
			m.view = ViewMain
		case "2":
			m.view = ViewWorkflows
		case "3":
			m.view = ViewTasks
		case "4":
			m.view = ViewConfig
		case "?", "h":
			m.view = ViewHelp

		case "up", "k":
			if m.view == ViewWorkflows && m.selectedIdx > 0 {
				m.selectedIdx--
			} else if m.view == ViewTasks && m.taskSelectedIdx > 0 {
				m.taskSelectedIdx--
			}

		case "down", "j":
			if m.view == ViewWorkflows && m.selectedIdx < len(m.workflows)-1 {
				m.selectedIdx++
			} else if m.view == ViewTasks && m.taskSelectedIdx < len(m.tasks)-1 {
				m.taskSelectedIdx++
			}

		case "enter":
			if m.view == ViewWorkflows && len(m.workflows) > 0 {
				// Show tasks for selected workflow
				m.tasks = m.workflows[m.selectedIdx].Tasks
				m.view = ViewTasks
				m.taskSelectedIdx = 0
			}

		case "esc":
			if m.view == ViewTasks {
				m.view = ViewWorkflows
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case workflowsLoadedMsg:
		m.workflows = msg.workflows
		m.loading = false

	case tasksLoadedMsg:
		m.tasks = msg.tasks
		m.loading = false

	case errMsg:
		m.err = msg.err
		m.loading = false

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var content string

	switch m.view {
	case ViewMain:
		content = m.viewMain()
	case ViewWorkflows:
		content = m.viewWorkflows()
	case ViewTasks:
		content = m.viewTasks()
	case ViewConfig:
		content = m.viewConfig()
	case ViewHelp:
		content = m.viewHelp()
	}

	return m.frame(content)
}

func (m Model) frame(content string) string {
	header := m.renderHeader()
	footer := m.renderFooter()

	// Calculate available height
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	contentHeight := m.height - headerHeight - footerHeight - 2

	// Ensure minimum height
	if contentHeight < 5 {
		contentHeight = 5
	}

	// Style the content area
	contentStyle := lipgloss.NewStyle().
		Height(contentHeight).
		Width(m.width - 4).
		Padding(1)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		contentStyle.Render(content),
		footer,
	)
}

func (m Model) renderHeader() string {
	title := TitleStyle.Render("Claude Orchestrator")
	tabs := m.renderTabs()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		tabs,
	)
}

func (m Model) renderTabs() string {
	tabs := []string{"[1] Main", "[2] Workflows", "[3] Tasks", "[4] Config", "[?] Help"}
	rendered := make([]string, len(tabs))

	for i, tab := range tabs {
		if ViewMode(i) == m.view || (i == 4 && m.view == ViewHelp) {
			rendered[i] = HeaderStyle.Render(tab)
		} else {
			rendered[i] = SubtitleStyle.Render(tab)
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m Model) renderFooter() string {
	help := HelpStyle.Render("q: quit • tab: switch view • ↑↓: navigate • enter: select")
	return help
}

func (m Model) viewMain() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Welcome to Claude Orchestrator"))
	b.WriteString("\n\n")

	// Status box
	statusBox := BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			SubtitleStyle.Render("Status"),
			fmt.Sprintf("  %s Provider: %s", IconBullet, "Claude Code"),
			fmt.Sprintf("  %s Workflows: %d", IconBullet, len(m.workflows)),
			fmt.Sprintf("  %s Active Tasks: %d", IconBullet, m.countActiveTasks()),
		),
	)

	b.WriteString(statusBox)
	b.WriteString("\n\n")

	// Quick actions
	b.WriteString(SubtitleStyle.Render("Quick Actions"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s Press [2] to view workflows\n", IconArrow))
	b.WriteString(fmt.Sprintf("  %s Press [3] to view tasks\n", IconArrow))
	b.WriteString(fmt.Sprintf("  %s Press [4] to view configuration\n", IconArrow))

	return b.String()
}

func (m Model) viewWorkflows() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Workflows"))
	b.WriteString("\n")

	if len(m.workflows) == 0 {
		b.WriteString(SubtitleStyle.Render("No workflows found"))
		return b.String()
	}

	// Render workflow list
	for i, wf := range m.workflows {
		var style lipgloss.Style
		if i == m.selectedIdx {
			style = TaskItemSelectedStyle
			b.WriteString(IconSelected + " ")
		} else {
			style = TaskItemStyle
			b.WriteString("  ")
		}

		statusStyle := GetStatusStyle(wf.Status)
		status := statusStyle.Render(fmt.Sprintf("[%s]", wf.Status))

		line := style.Render(fmt.Sprintf("%s %s - %s",
			wf.ID[:8],
			wf.Title,
			status,
		))
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) viewTasks() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Tasks"))
	b.WriteString("\n")

	if len(m.tasks) == 0 {
		b.WriteString(SubtitleStyle.Render("No tasks"))
		return b.String()
	}

	// Render task list
	for i, task := range m.tasks {
		var checkbox string
		switch task.Status {
		case "completed":
			checkbox = TaskCheckboxStyle.Render("[" + IconCheck + "]")
		case "failed":
			checkbox = ErrorStyle.Render("[" + IconCross + "]")
		case "in_progress":
			checkbox = StatusRunningStyle.Render("[" + IconRunning + "]")
		default:
			checkbox = TaskCheckboxEmptyStyle.Render("[" + IconPending + "]")
		}

		var style lipgloss.Style
		if i == m.taskSelectedIdx {
			style = TaskItemSelectedStyle
		} else {
			style = TaskItemStyle
		}

		agentStyle := GetAgentStyle(task.AgentType)
		agent := agentStyle.Render(task.AgentType)

		line := style.Render(fmt.Sprintf("%s %s (%s)", checkbox, task.Title, agent))
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("Press ESC to go back"))

	return b.String()
}

func (m Model) viewConfig() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Configuration"))
	b.WriteString("\n\n")

	// Provider section
	providerBox := BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			SubtitleStyle.Render("Claude Provider"),
			fmt.Sprintf("  Provider:     %s", m.getConfigValue("provider", "claude_code")),
			fmt.Sprintf("  Model:        %s", m.getConfigValue("model", "claude-sonnet-4-20250514")),
			fmt.Sprintf("  Working Dir:  %s", m.getConfigValue("working_dir", ".")),
		),
	)
	b.WriteString(providerBox)
	b.WriteString("\n\n")

	// Temporal section
	temporalBox := BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			SubtitleStyle.Render("Temporal"),
			fmt.Sprintf("  Address:    %s", m.getConfigValue("temporal_address", "localhost:7233")),
			fmt.Sprintf("  Namespace:  %s", m.getConfigValue("temporal_namespace", "default")),
			fmt.Sprintf("  TaskQueue:  %s", m.getConfigValue("task_queue", "claude-orchestrator")),
		),
	)
	b.WriteString(temporalBox)

	return b.String()
}

func (m Model) viewHelp() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Help"))
	b.WriteString("\n\n")

	help := `Navigation:
  1-4      Switch to specific view
  Tab      Cycle through views
  ↑/k      Move up
  ↓/j      Move down
  Enter    Select item
  ESC      Go back
  q        Quit

Views:
  [1] Main      - Dashboard with status overview
  [2] Workflows - List and manage workflows
  [3] Tasks     - View task progress
  [4] Config    - View configuration

Workflows:
  • Select a workflow and press Enter to see its tasks
  • Tasks show their status with checkboxes

Configuration:
  • Edit config.yaml to change settings
  • Set CLAUDE_PROVIDER environment variable to switch providers`

	b.WriteString(help)

	return b.String()
}

func (m Model) countActiveTasks() int {
	count := 0
	for _, task := range m.tasks {
		if task.Status == "in_progress" {
			count++
		}
	}
	return count
}

func (m Model) getConfigValue(key, defaultValue string) string {
	if val, ok := m.configValues[key]; ok {
		return val
	}
	return defaultValue
}

// SetWorkflows sets the workflows data.
func (m *Model) SetWorkflows(workflows []Workflow) {
	m.workflows = workflows
}

// SetTasks sets the tasks data.
func (m *Model) SetTasks(tasks []Task) {
	m.tasks = tasks
}

// SetConfigValues sets the configuration values for display.
func (m *Model) SetConfigValues(values map[string]string) {
	m.configValues = values
}
