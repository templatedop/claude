package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	m := NewModel()

	if m.view != ViewMain {
		t.Errorf("Expected initial view to be ViewMain, got %v", m.view)
	}

	if m.loading {
		t.Error("Expected loading to be false initially")
	}

	if m.quitting {
		t.Error("Expected quitting to be false initially")
	}

	if m.configValues == nil {
		t.Error("Expected configValues to be initialized")
	}
}

func TestModel_Init(t *testing.T) {
	m := NewModel()
	cmd := m.Init()

	if cmd == nil {
		t.Error("Expected Init to return a command")
	}
}

func TestModel_Update_Quit(t *testing.T) {
	m := NewModel()

	// Test 'q' key
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := newModel.(Model)

	if !model.quitting {
		t.Error("Expected model to be quitting after 'q' press")
	}

	if cmd == nil {
		t.Error("Expected quit command")
	}
}

func TestModel_Update_TabNavigation(t *testing.T) {
	m := NewModel()

	// Press tab to cycle views
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := newModel.(Model)

	if model.view != ViewWorkflows {
		t.Errorf("Expected view to be ViewWorkflows after tab, got %v", model.view)
	}

	// Press tab again
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = newModel.(Model)

	if model.view != ViewTasks {
		t.Errorf("Expected view to be ViewTasks after second tab, got %v", model.view)
	}
}

func TestModel_Update_NumberNavigation(t *testing.T) {
	m := NewModel()

	tests := []struct {
		key          string
		expectedView ViewMode
	}{
		{"1", ViewMain},
		{"2", ViewWorkflows},
		{"3", ViewTasks},
		{"4", ViewConfig},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			model := newModel.(Model)

			if model.view != tt.expectedView {
				t.Errorf("Expected view %v for key '%s', got %v", tt.expectedView, tt.key, model.view)
			}
		})
	}
}

func TestModel_Update_HelpView(t *testing.T) {
	m := NewModel()

	// Press '?' for help
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := newModel.(Model)

	if model.view != ViewHelp {
		t.Errorf("Expected view to be ViewHelp, got %v", model.view)
	}

	// Press 'h' should also show help
	m = NewModel()
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = newModel.(Model)

	if model.view != ViewHelp {
		t.Errorf("Expected view to be ViewHelp for 'h', got %v", model.view)
	}
}

func TestModel_Update_UpDownNavigation(t *testing.T) {
	m := NewModel()
	m.view = ViewWorkflows
	m.workflows = []Workflow{
		{ID: "1", Title: "Workflow 1"},
		{ID: "2", Title: "Workflow 2"},
		{ID: "3", Title: "Workflow 3"},
	}

	// Press down
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := newModel.(Model)

	if model.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx to be 1 after down, got %d", model.selectedIdx)
	}

	// Press down again
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = newModel.(Model)

	if model.selectedIdx != 2 {
		t.Errorf("Expected selectedIdx to be 2 after second down, got %d", model.selectedIdx)
	}

	// Press up
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = newModel.(Model)

	if model.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx to be 1 after up, got %d", model.selectedIdx)
	}
}

func TestModel_Update_VimNavigation(t *testing.T) {
	m := NewModel()
	m.view = ViewWorkflows
	m.workflows = []Workflow{
		{ID: "1", Title: "Workflow 1"},
		{ID: "2", Title: "Workflow 2"},
	}

	// Press 'j' (vim down)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := newModel.(Model)

	if model.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx to be 1 after 'j', got %d", model.selectedIdx)
	}

	// Press 'k' (vim up)
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = newModel.(Model)

	if model.selectedIdx != 0 {
		t.Errorf("Expected selectedIdx to be 0 after 'k', got %d", model.selectedIdx)
	}
}

func TestModel_Update_EnterSelectWorkflow(t *testing.T) {
	m := NewModel()
	m.view = ViewWorkflows
	m.workflows = []Workflow{
		{
			ID:    "1",
			Title: "Test Workflow",
			Tasks: []Task{
				{ID: "t1", Title: "Task 1"},
				{ID: "t2", Title: "Task 2"},
			},
		},
	}

	// Press enter to select workflow
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := newModel.(Model)

	if model.view != ViewTasks {
		t.Errorf("Expected view to be ViewTasks after enter, got %v", model.view)
	}

	if len(model.tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(model.tasks))
	}
}

func TestModel_Update_EscGoBack(t *testing.T) {
	m := NewModel()
	m.view = ViewTasks

	// Press escape to go back
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := newModel.(Model)

	if model.view != ViewWorkflows {
		t.Errorf("Expected view to be ViewWorkflows after escape, got %v", model.view)
	}
}

func TestModel_Update_WindowResize(t *testing.T) {
	m := NewModel()

	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	model := newModel.(Model)

	if model.width != 100 {
		t.Errorf("Expected width to be 100, got %d", model.width)
	}

	if model.height != 50 {
		t.Errorf("Expected height to be 50, got %d", model.height)
	}
}

func TestModel_View(t *testing.T) {
	m := NewModel()
	m.width = 80
	m.height = 24

	// Test each view
	views := []ViewMode{ViewMain, ViewWorkflows, ViewTasks, ViewConfig, ViewHelp}

	for _, view := range views {
		t.Run(view.String(), func(t *testing.T) {
			m.view = view
			output := m.View()

			if output == "" {
				t.Errorf("View %v rendered empty output", view)
			}
		})
	}
}

func TestModel_View_Quitting(t *testing.T) {
	m := NewModel()
	m.quitting = true

	output := m.View()

	if output != "" {
		t.Error("Expected empty output when quitting")
	}
}

func TestModel_SetWorkflows(t *testing.T) {
	m := NewModel()

	workflows := []Workflow{
		{ID: "1", Title: "Workflow 1"},
		{ID: "2", Title: "Workflow 2"},
	}

	m.SetWorkflows(workflows)

	if len(m.workflows) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(m.workflows))
	}
}

func TestModel_SetTasks(t *testing.T) {
	m := NewModel()

	tasks := []Task{
		{ID: "1", Title: "Task 1"},
		{ID: "2", Title: "Task 2"},
	}

	m.SetTasks(tasks)

	if len(m.tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(m.tasks))
	}
}

func TestModel_SetConfigValues(t *testing.T) {
	m := NewModel()

	values := map[string]string{
		"provider": "claude_code",
		"model":    "claude-sonnet-4",
	}

	m.SetConfigValues(values)

	if m.getConfigValue("provider", "") != "claude_code" {
		t.Error("Expected provider to be 'claude_code'")
	}
}

func TestModel_CountActiveTasks(t *testing.T) {
	m := NewModel()
	m.tasks = []Task{
		{ID: "1", Status: "in_progress"},
		{ID: "2", Status: "completed"},
		{ID: "3", Status: "in_progress"},
		{ID: "4", Status: "pending"},
	}

	count := m.countActiveTasks()

	if count != 2 {
		t.Errorf("Expected 2 active tasks, got %d", count)
	}
}

func TestTask_Fields(t *testing.T) {
	task := Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "A test task",
		Status:      "in_progress",
		AgentType:   "coder",
		StartTime:   time.Now(),
		Error:       "",
	}

	if task.ID != "task-123" {
		t.Errorf("Expected ID 'task-123', got '%s'", task.ID)
	}

	if task.AgentType != "coder" {
		t.Errorf("Expected AgentType 'coder', got '%s'", task.AgentType)
	}
}

func TestWorkflow_Fields(t *testing.T) {
	workflow := Workflow{
		ID:         "wf-123",
		Title:      "Test Workflow",
		Status:     "running",
		TokensUsed: 1000,
		Tasks: []Task{
			{ID: "t1", Title: "Task 1"},
		},
	}

	if workflow.ID != "wf-123" {
		t.Errorf("Expected ID 'wf-123', got '%s'", workflow.ID)
	}

	if len(workflow.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(workflow.Tasks))
	}
}

// Helper method for ViewMode
func (v ViewMode) String() string {
	switch v {
	case ViewMain:
		return "ViewMain"
	case ViewWorkflows:
		return "ViewWorkflows"
	case ViewTasks:
		return "ViewTasks"
	case ViewConfig:
		return "ViewConfig"
	case ViewHelp:
		return "ViewHelp"
	default:
		return "Unknown"
	}
}
