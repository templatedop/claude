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

// ExecutorWorkflow is a specialized workflow for executing commands and tests.
func ExecutorWorkflow(ctx workflow.Context, input AgentWorkflowInput) (*AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting executor workflow", "task_id", input.Task.ID)

	startTime := workflow.Now(ctx)

	// Set up activity options with longer timeout for execution
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute, // Longer timeout for builds/tests
		HeartbeatTimeout:    5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    2, // Fewer retries for execution
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	output := &AgentWorkflowOutput{
		AgentID:   fmt.Sprintf("executor-%s", input.Task.ID),
		TaskID:    input.Task.ID,
		Artifacts: []domain.Artifact{},
		Metadata:  make(map[string]interface{}),
	}

	// Determine execution type
	execType := "run" // default
	if et, ok := input.Task.Input["exec_type"].(string); ok {
		execType = et
	}

	var execResults []ExecutionResult
	totalTokens := 0

	switch execType {
	case "git_commit":
		// Git operations
		result, err := executeGitCommit(ctx, input)
		if err != nil {
			output.Status = string(domain.AgentStatusFailed)
			output.Error = err.Error()
			return output, err
		}
		execResults = append(execResults, result)

	case "git_push":
		result, err := executeGitPush(ctx, input)
		if err != nil {
			output.Status = string(domain.AgentStatusFailed)
			output.Error = err.Error()
			return output, err
		}
		execResults = append(execResults, result)

	default:
		// General execution - use Claude to determine what to do
		prompt := fmt.Sprintf(`You are a task executor. Analyze the following task and determine what needs to be executed.

Task: %s

Description:
%s

Based on the task, provide a structured response with:
1. The type of execution needed (build, test, deploy, etc.)
2. Specific steps to execute
3. Expected outcomes

Respond with JSON in this format:
{
  "exec_type": "build|test|deploy|run",
  "steps": [
    {"command": "command to run", "description": "what it does"}
  ],
  "success_criteria": "how to determine success"
}`, input.Task.Title, input.Task.Description)

		var completionResult *activity.CompletionResult
		err := workflow.ExecuteActivity(ctx, "Complete", activity.CompletionRequest{
			AgentConfig: input.AgentConfig,
			Prompt:      prompt,
		}).Get(ctx, &completionResult)

		if err != nil {
			logger.Error("Execution planning failed", "error", err)
			output.Status = string(domain.AgentStatusFailed)
			output.Error = err.Error()
			return output, err
		}

		totalTokens += completionResult.TokensUsed
		output.Output = completionResult.Response
	}

	// Compile results
	resultJSON, _ := json.MarshalIndent(execResults, "", "  ")
	if output.Output == "" {
		output.Output = string(resultJSON)
	} else {
		output.Output += "\n\nExecution Results:\n" + string(resultJSON)
	}

	output.Status = string(domain.AgentStatusCompleted)
	output.Metrics = AgentMetrics{
		TokensUsed:   totalTokens,
		APICallCount: 1,
		Duration:     workflow.Now(ctx).Sub(startTime),
	}

	// Store results in memory
	err := workflow.ExecuteActivity(ctx, "Store", activity.StoreMemoryRequest{
		Key:       fmt.Sprintf("execution:%s", input.Task.ID),
		Value:     output.Output,
		Namespace: "workflow",
		Tags:      []string{"execution", execType},
	}).Get(ctx, nil)

	if err != nil {
		logger.Warn("Failed to store execution results in memory", "error", err)
	}

	logger.Info("Executor workflow completed",
		"exec_type", execType,
		"duration", output.Metrics.Duration)

	return output, nil
}

// ExecutionResult represents the result of an execution step.
type ExecutionResult struct {
	Step    string `json:"step"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// executeGitCommit performs a git commit.
func executeGitCommit(ctx workflow.Context, input AgentWorkflowInput) (ExecutionResult, error) {
	logger := workflow.GetLogger(ctx)
	result := ExecutionResult{Step: "git_commit"}

	message := "Auto-commit by orchestrator"
	if msg, ok := input.Task.Input["message"].(string); ok {
		message = msg
	}

	dir := "."
	if d, ok := input.Task.Input["dir"].(string); ok {
		dir = d
	}

	var files []string
	if f, ok := input.Task.Input["files"].([]interface{}); ok {
		for _, file := range f {
			if s, ok := file.(string); ok {
				files = append(files, s)
			}
		}
	}

	// First check status
	var statusResult *activity.GitStatusResult
	err := workflow.ExecuteActivity(ctx, "Status", dir).Get(ctx, &statusResult)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to get git status: %v", err)
		return result, err
	}

	if statusResult.IsClean && len(files) == 0 {
		result.Success = true
		result.Output = "No changes to commit"
		return result, nil
	}

	// Commit
	var commitResult *activity.GitCommitResult
	err = workflow.ExecuteActivity(ctx, "Commit", activity.GitCommitRequest{
		Dir:     dir,
		Message: message,
		Files:   files,
		All:     len(files) == 0,
	}).Get(ctx, &commitResult)

	if err != nil {
		result.Error = fmt.Sprintf("Failed to commit: %v", err)
		return result, err
	}

	result.Success = true
	result.Output = fmt.Sprintf("Committed: %s (%s)", commitResult.Message, commitResult.Hash)
	logger.Info("Git commit successful", "hash", commitResult.Hash)

	return result, nil
}

// executeGitPush performs a git push.
func executeGitPush(ctx workflow.Context, input AgentWorkflowInput) (ExecutionResult, error) {
	logger := workflow.GetLogger(ctx)
	result := ExecutionResult{Step: "git_push"}

	dir := "."
	if d, ok := input.Task.Input["dir"].(string); ok {
		dir = d
	}

	remote := "origin"
	if r, ok := input.Task.Input["remote"].(string); ok {
		remote = r
	}

	branch := ""
	if b, ok := input.Task.Input["branch"].(string); ok {
		branch = b
	}

	var pushResult *activity.GitPushResult
	err := workflow.ExecuteActivity(ctx, "Push", activity.GitPushRequest{
		Dir:         dir,
		Remote:      remote,
		Branch:      branch,
		SetUpstream: true,
	}).Get(ctx, &pushResult)

	if err != nil {
		result.Error = fmt.Sprintf("Failed to push: %v", err)
		return result, err
	}

	result.Success = pushResult.Success
	result.Output = pushResult.Message
	logger.Info("Git push successful")

	return result, nil
}
