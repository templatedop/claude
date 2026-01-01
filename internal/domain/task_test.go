package domain

import (
	"testing"
)

func TestNewTask(t *testing.T) {
	task := NewTask("task-123", "Test Task", "A test task description", AgentTypeCoder)

	if task.ID != "task-123" {
		t.Errorf("Expected ID 'task-123', got '%s'", task.ID)
	}

	if task.Title != "Test Task" {
		t.Errorf("Expected Title 'Test Task', got '%s'", task.Title)
	}

	if task.Status != TaskStatusPending {
		t.Errorf("Expected Status 'pending', got '%s'", task.Status)
	}

	if task.Priority != TaskPriorityNormal {
		t.Errorf("Expected Priority Normal, got %d", task.Priority)
	}

	if task.AssignedTo != AgentTypeCoder {
		t.Errorf("Expected AssignedTo 'coder', got '%s'", task.AssignedTo)
	}

	if task.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", task.MaxRetries)
	}

	if task.Context.TimeoutSec != 300 {
		t.Errorf("Expected TimeoutSec 300, got %d", task.Context.TimeoutSec)
	}
}

func TestTask_IsTerminal(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		expected bool
	}{
		{TaskStatusPending, false},
		{TaskStatusQueued, false},
		{TaskStatusInProgress, false},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
		{TaskStatusBlocked, false},
	}

	for _, tt := range tests {
		task := &Task{Status: tt.status}
		if task.IsTerminal() != tt.expected {
			t.Errorf("IsTerminal() for status %s = %v, want %v", tt.status, task.IsTerminal(), tt.expected)
		}
	}
}

func TestTask_CanRetry(t *testing.T) {
	tests := []struct {
		status     TaskStatus
		retries    int
		maxRetries int
		expected   bool
	}{
		{TaskStatusFailed, 0, 3, true},
		{TaskStatusFailed, 2, 3, true},
		{TaskStatusFailed, 3, 3, false},
		{TaskStatusFailed, 4, 3, false},
		{TaskStatusCompleted, 0, 3, false},
		{TaskStatusPending, 0, 3, false},
	}

	for _, tt := range tests {
		task := &Task{
			Status:     tt.status,
			Retries:    tt.retries,
			MaxRetries: tt.maxRetries,
		}
		if task.CanRetry() != tt.expected {
			t.Errorf("CanRetry() for status=%s retries=%d max=%d = %v, want %v",
				tt.status, tt.retries, tt.maxRetries, task.CanRetry(), tt.expected)
		}
	}
}

func TestTaskQueue(t *testing.T) {
	queue := NewTaskQueue(10)

	if queue.Len() != 0 {
		t.Errorf("Expected empty queue, got len %d", queue.Len())
	}

	// Add tasks with different priorities
	task1 := &Task{ID: "1", Priority: TaskPriorityLow}
	task2 := &Task{ID: "2", Priority: TaskPriorityHigh}
	task3 := &Task{ID: "3", Priority: TaskPriorityNormal}
	task4 := &Task{ID: "4", Priority: TaskPriorityCritical}

	queue.Push(task1)
	queue.Push(task2)
	queue.Push(task3)
	queue.Push(task4)

	if queue.Len() != 4 {
		t.Errorf("Expected queue len 4, got %d", queue.Len())
	}

	// Pop should return highest priority first
	popped := queue.Pop()
	if popped.ID != "4" {
		t.Errorf("Expected task 4 (critical) first, got %s", popped.ID)
	}

	popped = queue.Pop()
	if popped.ID != "2" {
		t.Errorf("Expected task 2 (high) second, got %s", popped.ID)
	}

	popped = queue.Pop()
	if popped.ID != "3" {
		t.Errorf("Expected task 3 (normal) third, got %s", popped.ID)
	}

	popped = queue.Pop()
	if popped.ID != "1" {
		t.Errorf("Expected task 1 (low) last, got %s", popped.ID)
	}

	// Pop from empty queue
	popped = queue.Pop()
	if popped != nil {
		t.Errorf("Expected nil from empty queue, got %v", popped)
	}
}

func TestDefaultAgentConfigs(t *testing.T) {
	configs := DefaultAgentConfigs()

	expectedTypes := []AgentType{
		AgentTypeOrchestrator,
		AgentTypePlanner,
		AgentTypeResearcher,
		AgentTypeCoder,
		AgentTypeReviewer,
		AgentTypeExecutor,
	}

	for _, agentType := range expectedTypes {
		config, ok := configs[agentType]
		if !ok {
			t.Errorf("Missing config for agent type %s", agentType)
			continue
		}

		if config.Type != agentType {
			t.Errorf("Config type mismatch: expected %s, got %s", agentType, config.Type)
		}

		if config.Model == "" {
			t.Errorf("Config for %s has empty model", agentType)
		}

		if config.MaxTokens == 0 {
			t.Errorf("Config for %s has zero MaxTokens", agentType)
		}

		if config.SystemPrompt == "" {
			t.Errorf("Config for %s has empty SystemPrompt", agentType)
		}
	}
}
