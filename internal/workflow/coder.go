package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/activity"
	"github.com/anthropics/claude-orchestrator/internal/domain"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// CoderWorkflow is a specialized workflow for code generation tasks.
func CoderWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting coder workflow", "task_id", input.Task.ID)

	startTime := workflow.Now(ctx)

	// Set up activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    5 * time.Minute, // Increased for Claude Code CLI which can take longer
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	output := &AgentWorkflowOutput{
		AgentID:   fmt.Sprintf("coder-%s", input.Task.ID),
		TaskID:    input.Task.ID,
		Artifacts: []domain.Artifact{},
		Metadata:  make(map[string]interface{}),
	}

	totalTokens := 0
	totalAPICalls := 0

	// Retrieve any previous context (e.g., from planner or researcher)
	var planContext, researchContext string

	var planResult *activity.GetMemoryResult
	err := workflow.ExecuteActivity(ctx, "Get", activity.GetMemoryRequest{
		Key: fmt.Sprintf("workflow:%s:shared:plan:%s", workflow.GetInfo(ctx).WorkflowExecution.ID, input.Task.ParentID),
	}).Get(ctx, &planResult)
	if err == nil && planResult.Found {
		planContext = planResult.Value
	}

	var researchResult *activity.GetMemoryResult
	err = workflow.ExecuteActivity(ctx, "Get", activity.GetMemoryRequest{
		Key: fmt.Sprintf("workflow:%s:shared:research:%s", workflow.GetInfo(ctx).WorkflowExecution.ID, input.Task.ParentID),
	}).Get(ctx, &researchResult)
	if err == nil && researchResult.Found {
		researchContext = researchResult.Value
	}

	// Determine the programming language
	language := "go" // default
	if lang, ok := input.Task.Input["language"].(string); ok {
		language = lang
	}

	// Build code generation prompt
	prompt := fmt.Sprintf(`You are an expert %s developer. Generate production-ready code for the following task.

Task: %s

Description:
%s

Requirements:
1. Write clean, idiomatic %s code
2. Include proper error handling
3. Add appropriate comments
4. Follow best practices and design patterns
5. Make the code testable

`, language, input.Task.Title, input.Task.Description, language)

	if planContext != "" {
		prompt += fmt.Sprintf("\nImplementation Plan:\n%s\n", planContext)
	}

	if researchContext != "" {
		prompt += fmt.Sprintf("\nResearch Context:\n%s\n", researchContext)
	}

	// Add any additional context
	if len(input.Context) > 0 {
		contextJSON, _ := json.MarshalIndent(input.Context, "", "  ")
		prompt += fmt.Sprintf("\nAdditional Context:\n%s\n", string(contextJSON))
	}

	prompt += `
Please provide:
1. The main implementation code
2. Any helper functions or utilities
3. Example usage
4. Unit tests for the main functionality

Format your response with clear code blocks marked with the file path.`

	// Execute the Claude completion
	var completionResult *activity.CompletionResult
	err = workflow.ExecuteActivity(ctx, "Complete", activity.CompletionRequest{
		AgentConfig: input.AgentConfig,
		Prompt:      prompt,
	}).Get(ctx, &completionResult)

	if err != nil {
		logger.Error("Code generation failed", "error", err)
		output.Status = string(domain.AgentStatusFailed)
		output.Error = err.Error()
		return output, err
	}

	totalTokens += completionResult.TokensUsed
	totalAPICalls++

	output.Output = completionResult.Response

	// Parse code blocks from the response
	codeBlocks := parseCodeBlocks(completionResult.Response)
	for path, content := range codeBlocks {
		// Write the file
		var writeResult *activity.WriteFileResult
		err := workflow.ExecuteActivity(ctx, "WriteFile", activity.WriteFileRequest{
			Path:      path,
			Content:   content,
			CreateDir: true,
			Overwrite: true,
		}).Get(ctx, &writeResult)

		if err != nil {
			logger.Warn("Failed to write file", "path", path, "error", err)
		} else {
			output.Artifacts = append(output.Artifacts, domain.Artifact{
				Name:     path,
				Type:     "code",
				Path:     writeResult.Path,
				Content:  content,
				Checksum: writeResult.Checksum,
			})
			logger.Info("Wrote file", "path", writeResult.Path)
		}
	}

	output.Status = string(domain.AgentStatusCompleted)
	output.Metrics = AgentMetrics{
		TokensUsed:       totalTokens,
		PromptTokens:     completionResult.PromptTokens,
		CompletionTokens: completionResult.CompletionTokens,
		APICallCount:     totalAPICalls,
		Duration:         workflow.Now(ctx).Sub(startTime),
	}

	// Store code summary in memory
	err = workflow.ExecuteActivity(ctx, "Store", activity.StoreMemoryRequest{
		Key:       fmt.Sprintf("code:%s", input.Task.ID),
		Value:     output.Output,
		Namespace: "workflow",
		Tags:      []string{"code", language},
	}).Get(ctx, nil)

	if err != nil {
		logger.Warn("Failed to store code in memory", "error", err)
	}

	logger.Info("Coder workflow completed",
		"tokens", output.Metrics.TokensUsed,
		"artifacts", len(output.Artifacts),
		"duration", output.Metrics.Duration)

	return output, nil
}

// parseCodeBlocks extracts code blocks with file paths from the response.
func parseCodeBlocks(response string) map[string]string {
	blocks := make(map[string]string)

	lines := strings.Split(response, "\n")
	var currentPath string
	var currentContent strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		// Check for code block start with path
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block
				if currentPath != "" {
					blocks[currentPath] = strings.TrimSpace(currentContent.String())
				}
				currentPath = ""
				currentContent.Reset()
				inCodeBlock = false
			} else {
				// Start of code block - check for path in previous lines or in the line itself
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			currentContent.WriteString(line + "\n")
		} else {
			// Look for file path patterns
			if strings.HasPrefix(line, "File:") || strings.HasPrefix(line, "file:") {
				path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "File:"), "file:"))
				path = strings.Trim(path, "`")
				if path != "" {
					currentPath = path
				}
			} else if strings.HasPrefix(line, "//") && strings.Contains(line, ".") {
				// Check for path in comment like "// main.go"
				path := strings.TrimSpace(strings.TrimPrefix(line, "//"))
				if !strings.Contains(path, " ") && strings.Contains(path, ".") {
					currentPath = path
				}
			}
		}
	}

	return blocks
}
