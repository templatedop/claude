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

// ReviewerWorkflow is a specialized workflow for code review tasks.
func ReviewerWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting reviewer workflow", "task_id", input.Task.ID)

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
		AgentID:   fmt.Sprintf("reviewer-%s", input.Task.ID),
		TaskID:    input.Task.ID,
		Artifacts: []domain.Artifact{},
		Metadata:  make(map[string]interface{}),
	}

	// Collect code to review
	var codeToReview []string

	// Check if files are provided in input
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
					codeToReview = append(codeToReview, fmt.Sprintf("File: %s\n```\n%s\n```", filePath, readResult.Content))
				}
			}
		}
	}

	// Check context for agent results
	if agentResults, ok := input.Context["agent_results"].([]interface{}); ok {
		for _, r := range agentResults {
			if result, ok := r.(map[string]interface{}); ok {
				if output, ok := result["output"].(string); ok && output != "" {
					codeToReview = append(codeToReview, fmt.Sprintf("Agent Output:\n%s", output))
				}
			}
		}
	}

	// Build review prompt
	prompt := fmt.Sprintf(`You are a senior software engineer performing a code review. Review the following code/content thoroughly.

Task: %s

Description:
%s

`, input.Task.Title, input.Task.Description)

	if len(codeToReview) > 0 {
		prompt += "Code to Review:\n"
		for _, code := range codeToReview {
			prompt += "\n" + code + "\n"
		}
	} else {
		prompt += "\nNo code files found. Please review based on the task description."
	}

	prompt += `

Please provide a comprehensive review covering:

1. **Code Quality**
   - Readability and clarity
   - Code organization
   - Naming conventions

2. **Correctness**
   - Logic errors
   - Edge cases
   - Error handling

3. **Security**
   - Potential vulnerabilities
   - Input validation
   - Authentication/authorization issues

4. **Performance**
   - Inefficient algorithms
   - Resource usage
   - Optimization opportunities

5. **Maintainability**
   - Code duplication
   - Complexity
   - Test coverage

6. **Summary**
   - Overall assessment
   - Critical issues (if any)
   - Recommended actions

Format your review with clear sections and specific line references where applicable.`

	// Execute the Claude completion
	var completionResult *activity.CompletionResult
	err := workflow.ExecuteActivity(ctx, "Complete", activity.CompletionRequest{
		AgentConfig: input.AgentConfig,
		Prompt:      prompt,
	}).Get(ctx, &completionResult)

	if err != nil {
		logger.Error("Review failed", "error", err)
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

	// Parse review for issues
	issues := parseReviewIssues(completionResult.Response)
	output.Metadata["issues"] = issues
	output.Metadata["issue_count"] = len(issues)

	// Store review in memory
	err = workflow.ExecuteActivity(ctx, "Store", activity.StoreMemoryRequest{
		Key:       fmt.Sprintf("review:%s", input.Task.ID),
		Value:     output.Output,
		Namespace: "workflow",
		Tags:      []string{"review", "quality"},
	}).Get(ctx, nil)

	if err != nil {
		logger.Warn("Failed to store review in memory", "error", err)
	}

	// Create review artifact
	output.Artifacts = append(output.Artifacts, domain.Artifact{
		Name:    fmt.Sprintf("review-%s.md", input.Task.ID),
		Type:    "markdown",
		Content: output.Output,
	})

	logger.Info("Reviewer workflow completed",
		"tokens", output.Metrics.TokensUsed,
		"issues", len(issues),
		"duration", output.Metrics.Duration)

	return output, nil
}

// ReviewIssue represents an issue found during review.
type ReviewIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
}

// parseReviewIssues attempts to extract issues from the review.
func parseReviewIssues(review string) []ReviewIssue {
	var issues []ReviewIssue

	// Try to parse as JSON first
	var parsed struct {
		Issues []ReviewIssue `json:"issues"`
	}
	if err := json.Unmarshal([]byte(review), &parsed); err == nil {
		return parsed.Issues
	}

	// Otherwise, just return an empty list
	// In a real implementation, you could parse the markdown structure
	return issues
}
