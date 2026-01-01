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

// PlannerWorkflow is a specialized workflow for planning tasks.
func PlannerWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting planner workflow", "task_id", input.Task.ID)

	startTime := workflow.Now(ctx)

	// Set up activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	output := &AgentWorkflowOutput{
		AgentID:   fmt.Sprintf("planner-%s", input.Task.ID),
		TaskID:    input.Task.ID,
		Artifacts: []domain.Artifact{},
		Metadata:  make(map[string]interface{}),
	}

	// Build planning prompt
	prompt := fmt.Sprintf(`You are a software architect and planner. Analyze the following task and create a detailed implementation plan.

Task: %s

Description:
%s

Please provide:
1. A high-level architecture overview
2. Step-by-step implementation plan
3. Key components and their responsibilities
4. Dependencies and integration points
5. Potential risks and mitigation strategies
6. Estimated complexity for each step (low/medium/high)

Format your response as a structured plan with clear sections.`, input.Task.Title, input.Task.Description)

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
		logger.Error("Planning failed", "error", err)
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

	// Store plan in memory
	err = workflow.ExecuteActivity(ctx, "Store", activity.StoreMemoryRequest{
		Key:       fmt.Sprintf("plan:%s", input.Task.ID),
		Value:     output.Output,
		Namespace: "workflow",
		Tags:      []string{"plan", "architecture"},
	}).Get(ctx, nil)

	if err != nil {
		logger.Warn("Failed to store plan in memory", "error", err)
	}

	// Create plan artifact
	output.Artifacts = append(output.Artifacts, domain.Artifact{
		Name:    fmt.Sprintf("plan-%s.md", input.Task.ID),
		Type:    "markdown",
		Content: output.Output,
	})

	logger.Info("Planner workflow completed",
		"tokens", output.Metrics.TokensUsed,
		"duration", output.Metrics.Duration)

	return output, nil
}
