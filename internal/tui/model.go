package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/config"
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

// InputMode represents the current input state.
type InputMode int

const (
	InputNone InputMode = iota
	InputPrompt
	InputConfigPath
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

// SubmitHandler is called when a prompt is submitted.
type SubmitHandler func(prompt string) error

// Model is the main TUI model.
type Model struct {
	// Current view
	view ViewMode

	// Input mode
	inputMode InputMode

	// UI components
	spinner   spinner.Model
	textInput textinput.Model

	// Data
	workflows       []Workflow
	selectedIdx     int
	tasks           []Task
	taskSelectedIdx int

	// State
	loading      bool
	err          error
	width        int
	height       int
	quitting     bool
	statusMsg    string
	statusIsErr  bool

	// Config
	config       *config.Config
	configPath   string
	configKeys   []string
	configValues map[string]string

	// Callbacks
	onSubmit SubmitHandler
}

// Option is a functional option for configuring the Model.
type Option func(*Model)

// WithConfig sets the configuration for the TUI.
func WithConfig(cfg *config.Config) Option {
	return func(m *Model) {
		m.config = cfg
		m.updateConfigDisplay()
	}
}

// WithConfigPath sets the configuration file path.
func WithConfigPath(path string) Option {
	return func(m *Model) {
		m.configPath = path
	}
}

// WithSubmitHandler sets the callback for prompt submission.
func WithSubmitHandler(handler SubmitHandler) Option {
	return func(m *Model) {
		m.onSubmit = handler
	}
}

// NewModel creates a new TUI model.
func NewModel(opts ...Option) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	ti := textinput.New()
	ti.Placeholder = "Enter your prompt..."
	ti.CharLimit = 1000
	ti.Width = 60

	m := Model{
		view:         ViewMain,
		inputMode:    InputNone,
		spinner:      s,
		textInput:    ti,
		workflows:    []Workflow{},
		tasks:        []Task{},
		configValues: make(map[string]string),
		config:       config.DefaultConfig(),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m)
	}

	// Update config display
	m.updateConfigDisplay()

	return m
}

// updateConfigDisplay updates the config values for display.
func (m *Model) updateConfigDisplay() {
	if m.config == nil {
		return
	}

	m.configValues = map[string]string{
		"provider":           m.config.Claude.Provider,
		"model":              m.config.Claude.Model,
		"working_dir":        m.config.Claude.WorkingDir,
		"temporal_address":   m.config.Temporal.Address,
		"temporal_namespace": m.config.Temporal.Namespace,
		"task_queue":         m.config.Temporal.TaskQueue,
		"memory_type":        m.config.Memory.Type,
		"storage_type":       m.config.Storage.Type,
		"rag_enabled":        fmt.Sprintf("%v", m.config.RAG.Enabled),
		"mcp_enabled":        fmt.Sprintf("%v", m.config.MCP.Enabled),
		"lsp_enabled":        fmt.Sprintf("%v", m.config.LSP.Enabled),
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
	tickMsg        time.Time
	statusMsg      string
	configLoaded   struct{ cfg *config.Config }
	workflowSubmitted struct {
		id    string
		title string
	}
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Handle input mode first
	if m.inputMode != InputNone {
		return m.handleInputMode(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Clear status message on any key
		m.statusMsg = ""
		m.statusIsErr = false

		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "q":
			// Only quit if not in input mode
			if m.inputMode == InputNone {
				m.quitting = true
				return m, tea.Quit
			}

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

		case "n":
			// New prompt - enter input mode
			if m.view == ViewMain {
				m.inputMode = InputPrompt
				m.textInput.Reset()
				m.textInput.Placeholder = "Enter your prompt..."
				m.textInput.Focus()
				return m, textinput.Blink
			}

		case "c":
			// Load config - enter config path input mode
			if m.view == ViewConfig {
				m.inputMode = InputConfigPath
				m.textInput.Reset()
				m.textInput.Placeholder = "Enter config file path..."
				m.textInput.SetValue(m.configPath)
				m.textInput.Focus()
				return m, textinput.Blink
			}

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
		m.textInput.Width = m.width - 10
		if m.textInput.Width < 30 {
			m.textInput.Width = 30
		}

	case workflowsLoadedMsg:
		m.workflows = msg.workflows
		m.loading = false

	case tasksLoadedMsg:
		m.tasks = msg.tasks
		m.loading = false

	case configLoaded:
		m.config = msg.cfg
		m.updateConfigDisplay()
		m.statusMsg = "Config loaded successfully"
		m.statusIsErr = false
		m.loading = false

	case workflowSubmitted:
		// Add to workflows list
		m.workflows = append([]Workflow{{
			ID:        msg.id,
			Title:     msg.title,
			Status:    "running",
			StartTime: time.Now(),
		}}, m.workflows...)
		m.statusMsg = fmt.Sprintf("Workflow started: %s", msg.id[:8])
		m.statusIsErr = false
		m.loading = false

	case errMsg:
		m.err = msg.err
		m.statusMsg = msg.err.Error()
		m.statusIsErr = true
		m.loading = false

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleInputMode handles key events when in input mode.
func (m Model) handleInputMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "esc":
			// Cancel input mode
			m.inputMode = InputNone
			m.textInput.Blur()
			return m, nil

		case "enter":
			// Submit input
			value := strings.TrimSpace(m.textInput.Value())
			if value == "" {
				m.inputMode = InputNone
				m.textInput.Blur()
				return m, nil
			}

			switch m.inputMode {
			case InputPrompt:
				m.inputMode = InputNone
				m.textInput.Blur()
				return m.submitPrompt(value)

			case InputConfigPath:
				m.inputMode = InputNone
				m.textInput.Blur()
				return m.loadConfig(value)
			}

			m.inputMode = InputNone
			m.textInput.Blur()
			return m, nil
		}
	}

	// Update text input
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// submitPrompt handles prompt submission.
func (m Model) submitPrompt(prompt string) (tea.Model, tea.Cmd) {
	m.loading = true

	// If we have a submit handler, call it
	if m.onSubmit != nil {
		err := m.onSubmit(prompt)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			m.statusIsErr = true
			m.loading = false
			return m, nil
		}
	}

	// Create a local workflow entry
	workflowID := fmt.Sprintf("wf-%d", time.Now().UnixNano())
	m.workflows = append([]Workflow{{
		ID:        workflowID,
		Title:     truncateString(prompt, 50),
		Status:    "running",
		StartTime: time.Now(),
		Tasks: []Task{{
			ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
			Title:     prompt,
			Status:    "in_progress",
			AgentType: "orchestrator",
			StartTime: time.Now(),
		}},
	}}, m.workflows...)

	m.statusMsg = fmt.Sprintf("Workflow started: %s", workflowID[:10])
	m.statusIsErr = false
	m.loading = false

	return m, nil
}

// loadConfig loads configuration from a file.
func (m Model) loadConfig(path string) (tea.Model, tea.Cmd) {
	m.loading = true
	m.configPath = path

	cfg, err := config.LoadConfig(path)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Config error: %v", err)
		m.statusIsErr = true
		m.loading = false
		return m, nil
	}

	m.config = cfg
	m.updateConfigDisplay()
	m.statusMsg = "Config loaded: " + path
	m.statusIsErr = false
	m.loading = false

	return m, nil
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
	var parts []string

	// Show status message if present
	if m.statusMsg != "" {
		if m.statusIsErr {
			parts = append(parts, ErrorStyle.Render("Error: "+m.statusMsg))
		} else {
			parts = append(parts, SuccessStyle.Render(m.statusMsg))
		}
	}

	// Context-specific help
	var helpText string
	if m.inputMode != InputNone {
		helpText = "enter: submit • esc: cancel"
	} else {
		switch m.view {
		case ViewMain:
			helpText = "n: new prompt • q: quit • tab: switch view"
		case ViewConfig:
			helpText = "c: load config • q: quit • tab: switch view"
		case ViewWorkflows:
			helpText = "↑↓: navigate • enter: view tasks • q: quit"
		case ViewTasks:
			helpText = "↑↓: navigate • esc: back • q: quit"
		default:
			helpText = "q: quit • tab: switch view • ↑↓: navigate"
		}
	}

	parts = append(parts, HelpStyle.Render(helpText))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) viewMain() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Welcome to Claude Orchestrator"))
	b.WriteString("\n\n")

	// Show input mode if active
	if m.inputMode == InputPrompt {
		b.WriteString(SubtitleStyle.Render("New Workflow"))
		b.WriteString("\n\n")
		b.WriteString("Enter your prompt:\n")
		b.WriteString(BoxStyle.Render(m.textInput.View()))
		b.WriteString("\n\n")
		b.WriteString(HelpStyle.Render("Press Enter to submit, Esc to cancel"))
		return b.String()
	}

	// Status box
	provider := "api"
	if m.config != nil {
		provider = m.config.Claude.Provider
	}

	statusBox := BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			SubtitleStyle.Render("Status"),
			fmt.Sprintf("  %s Provider: %s", IconBullet, provider),
			fmt.Sprintf("  %s Workflows: %d", IconBullet, len(m.workflows)),
			fmt.Sprintf("  %s Active Tasks: %d", IconBullet, m.countActiveTasks()),
		),
	)

	b.WriteString(statusBox)
	b.WriteString("\n\n")

	// Prompt input section
	b.WriteString(SubtitleStyle.Render("Start New Workflow"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s Press [n] to enter a new prompt\n", IconArrow))
	b.WriteString("\n")

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

	// Show input mode if active
	if m.inputMode == InputConfigPath {
		b.WriteString(SubtitleStyle.Render("Load Configuration"))
		b.WriteString("\n\n")
		b.WriteString("Enter config file path:\n")
		b.WriteString(BoxStyle.Render(m.textInput.View()))
		b.WriteString("\n\n")
		b.WriteString(HelpStyle.Render("Press Enter to load, Esc to cancel"))
		return b.String()
	}

	// Config file info
	configPath := m.configPath
	if configPath == "" {
		configPath = "(default config)"
	}
	b.WriteString(fmt.Sprintf("Config: %s\n", HelpStyle.Render(configPath)))
	b.WriteString(fmt.Sprintf("  %s Press [c] to load a config file\n\n", IconArrow))

	// Provider section
	providerBox := BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			SubtitleStyle.Render("Claude Provider"),
			fmt.Sprintf("  Provider:     %s", m.getConfigValue("provider", "api")),
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
	b.WriteString("\n\n")

	// Features section
	featuresBox := BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			SubtitleStyle.Render("Features"),
			fmt.Sprintf("  RAG:       %s", m.getConfigValue("rag_enabled", "true")),
			fmt.Sprintf("  MCP:       %s", m.getConfigValue("mcp_enabled", "false")),
			fmt.Sprintf("  LSP:       %s", m.getConfigValue("lsp_enabled", "false")),
			fmt.Sprintf("  Memory:    %s", m.getConfigValue("memory_type", "inmemory")),
			fmt.Sprintf("  Storage:   %s", m.getConfigValue("storage_type", "local")),
		),
	)
	b.WriteString(featuresBox)

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
  ESC      Go back / Cancel
  q        Quit

Actions:
  n        New prompt (from Main view)
  c        Load config file (from Config view)

Views:
  [1] Main      - Dashboard and prompt entry
  [2] Workflows - List and manage workflows
  [3] Tasks     - View task progress
  [4] Config    - View and load configuration

Workflows:
  • Press 'n' on Main view to start a new workflow
  • Enter your prompt and press Enter to submit
  • Select a workflow and press Enter to see its tasks

Configuration:
  • Press 'c' on Config view to load a config file
  • Enter the path to your config.yaml or config.json
  • Config is also loaded via --config flag or CONFIG_PATH env`

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
