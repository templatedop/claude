// Package activity provides Temporal activity implementations.
package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/domain"
	"github.com/anthropics/claude-orchestrator/pkg/claude"
	"go.temporal.io/sdk/activity"
)

// ClaudeActivities contains activities for interacting with Claude.
type ClaudeActivities struct {
	client claude.ClaudeClient
}

// NewClaudeActivities creates a new ClaudeActivities instance using the API client.
// This uses the Anthropic API and requires API credits.
func NewClaudeActivities(apiKey string) (*ClaudeActivities, error) {
	client, err := claude.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Claude client: %w", err)
	}
	return &ClaudeActivities{client: client}, nil
}

// NewClaudeActivitiesWithClient creates a new ClaudeActivities with a custom client.
// This allows using either the API client or Claude Code client.
func NewClaudeActivitiesWithClient(client claude.ClaudeClient) *ClaudeActivities {
	return &ClaudeActivities{client: client}
}

// NewClaudeCodeActivities creates a new ClaudeActivities using Claude Code.
// This uses your Claude subscription instead of API credits.
// Requires Claude Code CLI to be installed and authenticated.
func NewClaudeCodeActivities(opts ...claude.ClaudeCodeOption) (*ClaudeActivities, error) {
	client, err := claude.NewClaudeCodeClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Claude Code client: %w", err)
	}
	return &ClaudeActivities{client: client}, nil
}

// Provider returns the provider type being used.
func (a *ClaudeActivities) Provider() claude.Provider {
	return a.client.Provider()
}

// Close cleans up any resources.
func (a *ClaudeActivities) Close() error {
	return a.client.Close()
}

// CompletionRequest represents a request for a Claude completion.
type CompletionRequest struct {
	AgentConfig domain.AgentConfig     `json:"agent_config"`
	Prompt      string                 `json:"prompt"`
	Context     string                 `json:"context,omitempty"`
	History     []claude.Message       `json:"history,omitempty"`
	Tools       []claude.ToolDefinition `json:"tools,omitempty"`
}

// CompletionResult represents the result of a Claude completion.
type CompletionResult struct {
	Response     string                 `json:"response"`
	ToolCalls    []claude.ToolUse       `json:"tool_calls,omitempty"`
	TokensUsed   int                    `json:"tokens_used"`
	PromptTokens int                    `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
	StopReason   string                 `json:"stop_reason"`
	Model        string                 `json:"model"`
	Duration     time.Duration          `json:"duration"`
}

// Complete performs a Claude completion using the configured provider.
func (a *ClaudeActivities) Complete(ctx context.Context, req CompletionRequest) (*CompletionResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Starting Claude completion", "agent", req.AgentConfig.Name, "provider", a.client.Provider())

	startTime := time.Now()

	// Build messages
	messages := make([]claude.Message, 0, len(req.History)+1)
	messages = append(messages, req.History...)

	// Add context if provided
	promptText := req.Prompt
	if req.Context != "" {
		promptText = fmt.Sprintf("Context:\n%s\n\nTask:\n%s", req.Context, req.Prompt)
	}
	messages = append(messages, claude.NewTextMessage(claude.RoleUser, promptText))

	// Build request parameters
	temp := req.AgentConfig.Temperature
	if temp == 0 {
		temp = 0.7
	}

	model := req.AgentConfig.Model
	if model == "" {
		model = claude.DefaultModel
	}

	maxTokens := req.AgentConfig.MaxTokens
	if maxTokens == 0 {
		maxTokens = claude.DefaultMaxTokens
	}

	// Build the unified completion request
	completionReq := claude.CompletionRequest{
		System:      req.AgentConfig.SystemPrompt,
		Prompt:      promptText,
		Model:       model,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Messages:    messages,
	}

	// Send heartbeat during potentially long operations
	activity.RecordHeartbeat(ctx, fmt.Sprintf("sending request to Claude (%s)", a.client.Provider()))

	resp, err := a.client.Complete(ctx, completionReq)
	if err != nil {
		logger.Error("Claude completion failed", "error", err, "provider", a.client.Provider())
		return nil, fmt.Errorf("Claude completion failed (%s): %w", a.client.Provider(), err)
	}

	duration := time.Since(startTime)

	result := &CompletionResult{
		Response:         resp.Text,
		ToolCalls:        resp.ToolUses,
		TokensUsed:       resp.InputTokens + resp.OutputTokens,
		PromptTokens:     resp.InputTokens,
		CompletionTokens: resp.OutputTokens,
		StopReason:       resp.StopReason,
		Model:            resp.Model,
		Duration:         duration,
	}

	logger.Info("Claude completion finished",
		"provider", a.client.Provider(),
		"tokens", result.TokensUsed,
		"duration", duration,
		"stop_reason", resp.StopReason)

	return result, nil
}

// DecomposeTaskRequest represents a request to decompose a task.
type DecomposeTaskRequest struct {
	Task        domain.Task          `json:"task"`
	AgentConfig domain.AgentConfig   `json:"agent_config"`
	MaxSubtasks int                  `json:"max_subtasks"`
}

// DecomposeTaskResult represents the result of task decomposition.
type DecomposeTaskResult struct {
	Subtasks    []domain.SubtaskDefinition `json:"subtasks"`
	Strategy    string                     `json:"strategy"`
	Rationale   string                     `json:"rationale"`
	TokensUsed  int                        `json:"tokens_used"`
}

// DecomposeTask breaks down a complex task into subtasks.
func (a *ClaudeActivities) DecomposeTask(ctx context.Context, req DecomposeTaskRequest) (*DecomposeTaskResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Decomposing task", "task_id", req.Task.ID, "title", req.Task.Title)

	maxSubtasks := req.MaxSubtasks
	if maxSubtasks == 0 {
		maxSubtasks = 5
	}

	prompt := fmt.Sprintf(`Analyze the following task and break it down into subtasks:

Task Title: %s
Task Description: %s

Requirements:
1. Create at most %d subtasks
2. Each subtask should be assignable to one of these agent types: planner, researcher, coder, reviewer, executor
3. Define clear dependencies between subtasks
4. Prioritize subtasks appropriately

Respond with a JSON object in this exact format:
{
  "subtasks": [
    {
      "type": "string (e.g., 'research', 'design', 'implement', 'review', 'test')",
      "title": "short title",
      "description": "detailed description",
      "assign_to": "agent type (planner/researcher/coder/reviewer/executor)",
      "priority": 1-4 (1=low, 4=critical),
      "dependencies": ["list of subtask indices that must complete first"],
      "input": {"any": "relevant input data"}
    }
  ],
  "strategy": "brief description of the overall approach",
  "rationale": "explanation of why this breakdown was chosen"
}`, req.Task.Title, req.Task.Description, maxSubtasks)

	activity.RecordHeartbeat(ctx, "decomposing task")

	completionReq := CompletionRequest{
		AgentConfig: req.AgentConfig,
		Prompt:      prompt,
	}

	result, err := a.Complete(ctx, completionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose task: %w", err)
	}

	// Parse the JSON response
	var decomposition struct {
		Subtasks  []domain.SubtaskDefinition `json:"subtasks"`
		Strategy  string                     `json:"strategy"`
		Rationale string                     `json:"rationale"`
	}

	if err := json.Unmarshal([]byte(result.Response), &decomposition); err != nil {
		// Try to extract JSON from the response
		jsonStart := findJSONStart(result.Response)
		if jsonStart >= 0 {
			jsonStr := result.Response[jsonStart:]
			if err := json.Unmarshal([]byte(jsonStr), &decomposition); err != nil {
				logger.Warn("Failed to parse decomposition JSON, using fallback", "error", err)
				// Return a single subtask as fallback
				decomposition.Subtasks = []domain.SubtaskDefinition{
					{
						Type:        req.Task.Type,
						Title:       req.Task.Title,
						Description: req.Task.Description,
						AssignTo:    domain.AgentTypeCoder,
						Priority:    req.Task.Priority,
					},
				}
				decomposition.Strategy = "direct execution"
				decomposition.Rationale = "Task could not be decomposed, executing directly"
			}
		}
	}

	return &DecomposeTaskResult{
		Subtasks:   decomposition.Subtasks,
		Strategy:   decomposition.Strategy,
		Rationale:  decomposition.Rationale,
		TokensUsed: result.TokensUsed,
	}, nil
}

// AnalyzeCodeRequest represents a request to analyze code.
type AnalyzeCodeRequest struct {
	Code        string             `json:"code"`
	Language    string             `json:"language"`
	Task        string             `json:"task"` // "review", "explain", "improve", "debug"
	AgentConfig domain.AgentConfig `json:"agent_config"`
}

// AnalyzeCodeResult represents the result of code analysis.
type AnalyzeCodeResult struct {
	Analysis   string   `json:"analysis"`
	Issues     []Issue  `json:"issues,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	TokensUsed int      `json:"tokens_used"`
}

// Issue represents a code issue found during analysis.
type Issue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// AnalyzeCode performs code analysis using Claude.
func (a *ClaudeActivities) AnalyzeCode(ctx context.Context, req AnalyzeCodeRequest) (*AnalyzeCodeResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Analyzing code", "language", req.Language, "task", req.Task)

	var prompt string
	switch req.Task {
	case "review":
		prompt = fmt.Sprintf(`Review the following %s code for:
1. Bugs and errors
2. Security vulnerabilities
3. Performance issues
4. Code style and best practices
5. Potential improvements

Code:
%s

Respond with a JSON object:
{
  "analysis": "overall summary",
  "issues": [
    {"type": "bug|security|performance|style", "severity": "high|medium|low", "line": 0, "description": "...", "suggestion": "..."}
  ],
  "suggestions": ["improvement suggestions"]
}`, req.Language, req.Code)

	case "explain":
		prompt = fmt.Sprintf(`Explain the following %s code in detail:
- What it does
- How it works
- Key components and their purposes
- Any notable patterns or techniques used

Code:
%s`, req.Language, req.Code)

	case "improve":
		prompt = fmt.Sprintf(`Suggest improvements for the following %s code:
- Code quality improvements
- Performance optimizations
- Better error handling
- Modern language features that could be used

Code:
%s

Provide specific code examples for each improvement.`, req.Language, req.Code)

	case "debug":
		prompt = fmt.Sprintf(`Debug the following %s code:
- Identify potential bugs
- Trace the execution flow
- Find logic errors
- Suggest fixes

Code:
%s`, req.Language, req.Code)

	default:
		prompt = fmt.Sprintf(`Analyze the following %s code:\n\n%s`, req.Language, req.Code)
	}

	activity.RecordHeartbeat(ctx, "analyzing code")

	completionReq := CompletionRequest{
		AgentConfig: req.AgentConfig,
		Prompt:      prompt,
	}

	result, err := a.Complete(ctx, completionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze code: %w", err)
	}

	analysisResult := &AnalyzeCodeResult{
		Analysis:   result.Response,
		TokensUsed: result.TokensUsed,
	}

	// Try to parse JSON if present
	if req.Task == "review" {
		var parsed struct {
			Analysis    string   `json:"analysis"`
			Issues      []Issue  `json:"issues"`
			Suggestions []string `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(result.Response), &parsed); err == nil {
			analysisResult.Analysis = parsed.Analysis
			analysisResult.Issues = parsed.Issues
			analysisResult.Suggestions = parsed.Suggestions
		}
	}

	return analysisResult, nil
}

// GenerateCodeRequest represents a request to generate code.
type GenerateCodeRequest struct {
	Specification string             `json:"specification"`
	Language      string             `json:"language"`
	Context       string             `json:"context,omitempty"`
	Style         string             `json:"style,omitempty"`
	AgentConfig   domain.AgentConfig `json:"agent_config"`
}

// GenerateCodeResult represents the result of code generation.
type GenerateCodeResult struct {
	Code        string   `json:"code"`
	Explanation string   `json:"explanation"`
	Files       []File   `json:"files,omitempty"`
	TokensUsed  int      `json:"tokens_used"`
}

// File represents a generated file.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// GenerateCode generates code based on specifications.
func (a *ClaudeActivities) GenerateCode(ctx context.Context, req GenerateCodeRequest) (*GenerateCodeResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Generating code", "language", req.Language)

	prompt := fmt.Sprintf(`Generate %s code based on the following specification:

Specification:
%s`, req.Language, req.Specification)

	if req.Context != "" {
		prompt += fmt.Sprintf("\n\nContext:\n%s", req.Context)
	}

	if req.Style != "" {
		prompt += fmt.Sprintf("\n\nStyle guidelines:\n%s", req.Style)
	}

	prompt += `

Respond with a JSON object:
{
  "code": "the main code",
  "explanation": "brief explanation of the implementation",
  "files": [
    {"path": "relative/path/to/file.ext", "content": "file content"}
  ]
}`

	activity.RecordHeartbeat(ctx, "generating code")

	completionReq := CompletionRequest{
		AgentConfig: req.AgentConfig,
		Prompt:      prompt,
	}

	result, err := a.Complete(ctx, completionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate code: %w", err)
	}

	codeResult := &GenerateCodeResult{
		Code:       result.Response,
		TokensUsed: result.TokensUsed,
	}

	// Try to parse JSON
	var parsed struct {
		Code        string `json:"code"`
		Explanation string `json:"explanation"`
		Files       []File `json:"files"`
	}
	if err := json.Unmarshal([]byte(result.Response), &parsed); err == nil {
		codeResult.Code = parsed.Code
		codeResult.Explanation = parsed.Explanation
		codeResult.Files = parsed.Files
	}

	return codeResult, nil
}

// findJSONStart finds the start of a JSON object in a string.
func findJSONStart(s string) int {
	for i, c := range s {
		if c == '{' {
			return i
		}
	}
	return -1
}
