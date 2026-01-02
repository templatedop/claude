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

// ResearcherWorkflow is a specialized workflow for research tasks.
func ResearcherWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting researcher workflow", "task_id", input.Task.ID)

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
		AgentID:   fmt.Sprintf("researcher-%s", input.Task.ID),
		TaskID:    input.Task.ID,
		Artifacts: []domain.Artifact{},
		Metadata:  make(map[string]interface{}),
	}

	// Check if we need to read files for context
	var fileContents []string
	if files, ok := input.Task.Input["files"].([]interface{}); ok {
		for _, f := range files {
			if filePath, ok := f.(string); ok {
				var readResult *activity.ReadFileResult
				err := workflow.ExecuteActivity(ctx, "ReadFile", activity.ReadFileRequest{
					Path: filePath,
				}).Get(ctx, &readResult)

				if err != nil {
					logger.Warn("Failed to read file", "path", filePath, "error", err)
				} else {
					fileContents = append(fileContents, fmt.Sprintf("File: %s\n```\n%s\n```", filePath, readResult.Content))
				}
			}
		}
	}

	// Build research prompt
	prompt := fmt.Sprintf(`You are a technical researcher. Analyze the following task and gather relevant information.

Task: %s

Description:
%s

Please provide:
1. Key concepts and technologies involved
2. Best practices and patterns to follow
3. Potential approaches and their trade-offs
4. Relevant examples or references
5. Recommendations for implementation

Be thorough and cite specific patterns or practices when applicable.`, input.Task.Title, input.Task.Description)

	// Add file contents if available
	if len(fileContents) > 0 {
		prompt += "\n\nExisting Code/Files:\n"
		for _, content := range fileContents {
			prompt += "\n" + content + "\n"
		}
	}

	// Add context if available
	if len(input.Context) > 0 {
		contextJSON, _ := json.MarshalIndent(input.Context, "", "  ")
		prompt += fmt.Sprintf("\n\nAdditional Context:\n%s", string(contextJSON))
	}

	// Execute the Claude completion
	var completionResult *activity.CompletionResult
	err := workflow.ExecuteActivity(ctx, "Complete", activity.CompletionRequest{
		AgentConfig: input.AgentConfig,
		Prompt:      prompt,
	}).Get(ctx, &completionResult)

	if err != nil {
		logger.Error("Research failed", "error", err)
		output.Status = string(domain.AgentStatusFailed)
		output.Error = err.Error()
		return output, err
	}

	output.Output = completionResult.Response
	output.Status = string(domain.AgentStatusCompleted)
	output.Metrics = AgentMetrics{
		TokensUsed:       completionResult.TokensUsed,
		PromptTokens:     completionResult.PromptTokens,
		CompletionTokens: completionResult.CompletionTokens,
		APICallCount:     1,
		Duration:         workflow.Now(ctx).Sub(startTime),
	}

	// Store research in memory
	err = workflow.ExecuteActivity(ctx, "Store", activity.StoreMemoryRequest{
		Key:       fmt.Sprintf("research:%s", input.Task.ID),
		Value:     output.Output,
		Namespace: "workflow",
		Tags:      []string{"research", "context"},
	}).Get(ctx, nil)

	if err != nil {
		logger.Warn("Failed to store research in memory", "error", err)
	}

	// Create research artifact
	output.Artifacts = append(output.Artifacts, domain.Artifact{
		Name:    fmt.Sprintf("research-%s.md", input.Task.ID),
		Type:    "markdown",
		Content: output.Output,
	})

	logger.Info("Researcher workflow completed",
		"tokens", output.Metrics.TokensUsed,
		"duration", output.Metrics.Duration)

	return output, nil
}
