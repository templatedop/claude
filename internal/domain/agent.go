// Package domain contains core domain types for the orchestrator.
package domain

import "time"

// AgentType represents the type of agent in the swarm.
type AgentType string

const (
	AgentTypeOrchestrator AgentType = "orchestrator"
	AgentTypePlanner      AgentType = "planner"
	AgentTypeResearcher   AgentType = "researcher"
	AgentTypeCoder        AgentType = "coder"
	AgentTypeReviewer     AgentType = "reviewer"
	AgentTypeExecutor     AgentType = "executor"
)

// AgentStatus represents the current status of an agent.
type AgentStatus string

const (
	AgentStatusPending   AgentStatus = "pending"
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusBlocked   AgentStatus = "blocked"
)

// AgentConfig holds configuration for an agent.
type AgentConfig struct {
	ID           string            `json:"id"`
	Type         AgentType         `json:"type"`
	Name         string            `json:"name"`
	SystemPrompt string            `json:"system_prompt"`
	Model        string            `json:"model"`
	MaxTokens    int               `json:"max_tokens"`
	Temperature  float64           `json:"temperature"`
	Tools        []string          `json:"tools"`
	Metadata     map[string]string `json:"metadata"`
}

// Agent represents an agent instance in the swarm.
type Agent struct {
	Config      AgentConfig `json:"config"`
	WorkflowID  string      `json:"workflow_id"`
	RunID       string      `json:"run_id"`
	Status      AgentStatus `json:"status"`
	ParentID    string      `json:"parent_id"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
}

// AgentResult represents the result of an agent's execution.
type AgentResult struct {
	AgentID      string                 `json:"agent_id"`
	TaskID       string                 `json:"task_id"`
	Status       AgentStatus            `json:"status"`
	Output       string                 `json:"output"`
	Artifacts    []Artifact             `json:"artifacts"`
	Metrics      AgentMetrics           `json:"metrics"`
	Error        string                 `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
	CompletedAt  time.Time              `json:"completed_at"`
}

// Artifact represents a file or resource produced by an agent.
type Artifact struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Path     string `json:"path"`
	Content  string `json:"content,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

// AgentMetrics contains performance metrics for an agent.
type AgentMetrics struct {
	TokensUsed      int           `json:"tokens_used"`
	PromptTokens    int           `json:"prompt_tokens"`
	CompletionTokens int          `json:"completion_tokens"`
	APICallCount    int           `json:"api_call_count"`
	Duration        time.Duration `json:"duration"`
	MemoryReads     int           `json:"memory_reads"`
	MemoryWrites    int           `json:"memory_writes"`
}

// DefaultAgentConfigs returns default configurations for each agent type.
func DefaultAgentConfigs() map[AgentType]AgentConfig {
	return map[AgentType]AgentConfig{
		AgentTypeOrchestrator: {
			Type:        AgentTypeOrchestrator,
			Name:        "Orchestrator",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   4096,
			Temperature: 0.3,
			SystemPrompt: `You are the Orchestrator (Queen) agent responsible for:
1. Decomposing complex tasks into subtasks
2. Assigning tasks to specialized agents
3. Coordinating agent communication
4. Aggregating results and ensuring quality
5. Managing the overall workflow

Be strategic, efficient, and ensure all subtasks are properly delegated.`,
		},
		AgentTypePlanner: {
			Type:        AgentTypePlanner,
			Name:        "Planner",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   4096,
			Temperature: 0.4,
			SystemPrompt: `You are the Planner agent responsible for:
1. Analyzing requirements and constraints
2. Creating detailed implementation plans
3. Identifying dependencies between tasks
4. Estimating complexity and risks
5. Designing system architecture

Provide clear, actionable plans with specific steps.`,
		},
		AgentTypeResearcher: {
			Type:        AgentTypeResearcher,
			Name:        "Researcher",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   4096,
			Temperature: 0.5,
			SystemPrompt: `You are the Researcher agent responsible for:
1. Gathering information about technologies and patterns
2. Analyzing existing codebases
3. Finding relevant documentation
4. Identifying best practices
5. Providing context for implementation

Be thorough and cite sources when applicable.`,
		},
		AgentTypeCoder: {
			Type:        AgentTypeCoder,
			Name:        "Coder",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   8192,
			Temperature: 0.2,
			SystemPrompt: `You are the Coder agent responsible for:
1. Writing clean, efficient, production-ready code
2. Following best practices and coding standards
3. Implementing proper error handling
4. Writing tests alongside code
5. Documenting code appropriately

Focus on correctness, readability, and maintainability.`,
		},
		AgentTypeReviewer: {
			Type:        AgentTypeReviewer,
			Name:        "Reviewer",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   4096,
			Temperature: 0.3,
			SystemPrompt: `You are the Reviewer agent responsible for:
1. Reviewing code for bugs and issues
2. Checking for security vulnerabilities
3. Ensuring code follows best practices
4. Verifying tests are adequate
5. Suggesting improvements

Be constructive and specific in your feedback.`,
		},
		AgentTypeExecutor: {
			Type:        AgentTypeExecutor,
			Name:        "Executor",
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   2048,
			Temperature: 0.1,
			SystemPrompt: `You are the Executor agent responsible for:
1. Running commands and scripts
2. Executing tests
3. Building and deploying code
4. Monitoring execution results
5. Reporting outcomes

Be precise and handle errors gracefully.`,
		},
	}
}
