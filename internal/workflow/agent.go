package workflow

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/activity"
	"github.com/anthropics/claude-orchestrator/internal/domain"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// AgentWorkflowInput represents the input to an agent workflow.
type AgentWorkflowInput struct {
	Task        domain.Task            `json:"task"`
	AgentConfig domain.AgentConfig     `json:"agent_config"`
	Context     map[string]interface{} `json:"context,omitempty"`
	History     []Message              `json:"history,omitempty"`
}

// Message represents a conversation message for agent context.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AgentWorkflowOutput represents the output of an agent workflow.
type AgentWorkflowOutput struct {
	AgentID    string           `json:"agent_id"`
	TaskID     string           `json:"task_id"`
	Status     string           `json:"status"`
	Output     string           `json:"output"`
	Artifacts  []domain.Artifact `json:"artifacts"`
	Metrics    AgentMetrics     `json:"metrics"`
	Error      string           `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// AgentMetrics contains metrics for an agent's execution.
type AgentMetrics struct {
	TokensUsed       int           `json:"tokens_used"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	APICallCount     int           `json:"api_call_count"`
	Duration         time.Duration `json:"duration"`
}

// AgentWorkflow is a generic agent workflow.
func AgentWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting agent workflow",
		"agent_type", input.AgentConfig.Type,
		"task_id", input.Task.ID,
		"task_title", input.Task.Title)

	startTime := workflow.Now(ctx)

	// Set up activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    5 * time.Minute, // Increased for Claude Code CLI
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	output := &AgentWorkflowOutput{
		AgentID:   input.AgentConfig.ID,
		TaskID:    input.Task.ID,
		Artifacts: []domain.Artifact{},
		Metadata:  make(map[string]interface{}),
	}

	// Build the prompt
	prompt := buildPrompt(input)

	// Execute the Claude completion
	var completionResult *activity.CompletionResult
	err := workflow.ExecuteActivity(ctx, "Complete", activity.CompletionRequest{
		AgentConfig: input.AgentConfig,
		Prompt:      prompt,
		Context:     formatContext(input.Context),
	}).Get(ctx, &completionResult)

	if err != nil {
		logger.Error("Claude completion failed", "error", err)
		output.Status = string(domain.AgentStatusFailed)
		output.Error = err.Error()
		return output, err
	}

	// Process the result
	output.Output = completionResult.Response
	output.Status = string(domain.AgentStatusCompleted)
	output.Metrics = AgentMetrics{
		TokensUsed:       completionResult.TokensUsed,
		PromptTokens:     completionResult.PromptTokens,
		CompletionTokens: completionResult.CompletionTokens,
		APICallCount:     1,
		Duration:         workflow.Now(ctx).Sub(startTime),
	}

	// Handle tool calls if present
	if len(completionResult.ToolCalls) > 0 {
		logger.Info("Processing tool calls", "count", len(completionResult.ToolCalls))

		for _, toolCall := range completionResult.ToolCalls {
			result, err := executeToolCall(ctx, toolCall)
			if err != nil {
				logger.Warn("Tool call failed", "tool", toolCall.Name, "error", err)
			} else {
				output.Metadata[toolCall.Name] = result
			}
		}
	}

	// Store result in memory
	err = workflow.ExecuteActivity(ctx, "Store", activity.StoreMemoryRequest{
		Key:       fmt.Sprintf("result:%s", input.Task.ID),
		Value:     output.Output,
		Namespace: "workflow",
		Tags:      []string{"agent_result", string(input.AgentConfig.Type)},
	}).Get(ctx, nil)

	if err != nil {
		logger.Warn("Failed to store result in memory", "error", err)
	}

	logger.Info("Agent workflow completed",
		"status", output.Status,
		"tokens", output.Metrics.TokensUsed,
		"duration", output.Metrics.Duration)

	return output, nil
}

// buildPrompt constructs the prompt for the agent.
func buildPrompt(input AgentWorkflowInput) string {
	prompt := fmt.Sprintf("Task: %s\n\nDescription:\n%s", input.Task.Title, input.Task.Description)

	if len(input.Task.Input) > 0 {
		inputJSON, _ := json.MarshalIndent(input.Task.Input, "", "  ")
		prompt += fmt.Sprintf("\n\nInput:\n%s", string(inputJSON))
	}

	return prompt
}

// formatContext formats the context map as a string.
func formatContext(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return ""
	}
	contextJSON, _ := json.MarshalIndent(ctx, "", "  ")
	return string(contextJSON)
}

// executeToolCall executes a tool call and returns the result.
func executeToolCall(ctx workflow.Context, toolCall interface{}) (interface{}, error) {
	// Tool call execution would be implemented here
	// This is a placeholder for the tool execution logic
	return nil, nil
}

// Signal channel names
const (
	SignalTaskComplete = "task_complete"
	SignalTaskFailed   = "task_failed"
	SignalStatusUpdate = "status_update"
	SignalShutdown     = "shutdown"
)

// AgentState represents the state of an agent during execution.
type AgentState struct {
	Status     domain.AgentStatus
	Progress   float64
	LastUpdate time.Time
	Error      string
}

// QueryAgentState queries the current state of an agent.
func QueryAgentState(ctx workflow.Context) (*AgentState, error) {
	// This would be used with workflow.SetQueryHandler
	return &AgentState{
		Status:     domain.AgentStatusRunning,
		Progress:   0.5,
		LastUpdate: workflow.Now(ctx),
	}, nil
}
