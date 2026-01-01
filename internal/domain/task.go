package domain

import "time"

// TaskPriority represents the priority level of a task.
type TaskPriority int

const (
	TaskPriorityLow    TaskPriority = 1
	TaskPriorityNormal TaskPriority = 2
	TaskPriorityHigh   TaskPriority = 3
	TaskPriorityCritical TaskPriority = 4
)

// TaskStatus represents the current status of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
	TaskStatusBlocked    TaskStatus = "blocked"
)

// Task represents a unit of work to be performed by an agent.
type Task struct {
	ID           string                 `json:"id"`
	ParentID     string                 `json:"parent_id,omitempty"`
	Type         string                 `json:"type"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Status       TaskStatus             `json:"status"`
	Priority     TaskPriority           `json:"priority"`
	AssignedTo   AgentType              `json:"assigned_to"`
	Dependencies []string               `json:"dependencies"`
	Input        map[string]interface{} `json:"input"`
	Output       map[string]interface{} `json:"output,omitempty"`
	Context      TaskContext            `json:"context"`
	Metadata     map[string]string      `json:"metadata"`
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Retries      int                    `json:"retries"`
	MaxRetries   int                    `json:"max_retries"`
}

// TaskContext provides additional context for task execution.
type TaskContext struct {
	WorkflowID    string            `json:"workflow_id"`
	SessionID     string            `json:"session_id"`
	WorkingDir    string            `json:"working_dir"`
	Environment   map[string]string `json:"environment"`
	MemoryKeys    []string          `json:"memory_keys"`
	AllowedTools  []string          `json:"allowed_tools"`
	TimeoutSec    int               `json:"timeout_sec"`
}

// TaskResult represents the outcome of task execution.
type TaskResult struct {
	TaskID      string                 `json:"task_id"`
	Status      TaskStatus             `json:"status"`
	Output      map[string]interface{} `json:"output"`
	Artifacts   []Artifact             `json:"artifacts"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	TokensUsed  int                    `json:"tokens_used"`
	ChildTasks  []string               `json:"child_tasks,omitempty"`
}

// TaskDecomposition represents a breakdown of a complex task.
type TaskDecomposition struct {
	OriginalTask Task     `json:"original_task"`
	Subtasks     []Task   `json:"subtasks"`
	Strategy     string   `json:"strategy"`
	Rationale    string   `json:"rationale"`
}

// SubtaskDefinition defines a subtask during decomposition.
type SubtaskDefinition struct {
	Type         string            `json:"type"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	AssignTo     AgentType         `json:"assign_to"`
	Priority     TaskPriority      `json:"priority"`
	Dependencies []string          `json:"dependencies"`
	Input        map[string]interface{} `json:"input"`
}

// NewTask creates a new task with default values.
func NewTask(id, title, description string, agentType AgentType) *Task {
	now := time.Now()
	return &Task{
		ID:           id,
		Title:        title,
		Description:  description,
		Status:       TaskStatusPending,
		Priority:     TaskPriorityNormal,
		AssignedTo:   agentType,
		Dependencies: []string{},
		Input:        make(map[string]interface{}),
		Metadata:     make(map[string]string),
		CreatedAt:    now,
		MaxRetries:   3,
		Context: TaskContext{
			Environment:  make(map[string]string),
			MemoryKeys:   []string{},
			AllowedTools: []string{},
			TimeoutSec:   300, // 5 minutes default
		},
	}
}

// IsTerminal returns true if the task is in a terminal state.
func (t *Task) IsTerminal() bool {
	return t.Status == TaskStatusCompleted ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusCancelled
}

// CanRetry returns true if the task can be retried.
func (t *Task) CanRetry() bool {
	return t.Status == TaskStatusFailed && t.Retries < t.MaxRetries
}

// TaskQueue represents a priority queue of tasks.
type TaskQueue struct {
	Tasks    []*Task `json:"tasks"`
	Capacity int     `json:"capacity"`
}

// NewTaskQueue creates a new task queue.
func NewTaskQueue(capacity int) *TaskQueue {
	return &TaskQueue{
		Tasks:    make([]*Task, 0, capacity),
		Capacity: capacity,
	}
}

// Push adds a task to the queue.
func (q *TaskQueue) Push(task *Task) {
	q.Tasks = append(q.Tasks, task)
	// Sort by priority (higher first)
	for i := len(q.Tasks) - 1; i > 0; i-- {
		if q.Tasks[i].Priority > q.Tasks[i-1].Priority {
			q.Tasks[i], q.Tasks[i-1] = q.Tasks[i-1], q.Tasks[i]
		} else {
			break
		}
	}
}

// Pop removes and returns the highest priority task.
func (q *TaskQueue) Pop() *Task {
	if len(q.Tasks) == 0 {
		return nil
	}
	task := q.Tasks[0]
	q.Tasks = q.Tasks[1:]
	return task
}

// Len returns the number of tasks in the queue.
func (q *TaskQueue) Len() int {
	return len(q.Tasks)
}
