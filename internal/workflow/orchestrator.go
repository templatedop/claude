// Package workflow provides Temporal workflow implementations.
package workflow

import (
	"fmt"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/activity"
	"github.com/anthropics/claude-orchestrator/internal/domain"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// TaskQueueName is the default task queue for the orchestrator.
	TaskQueueName = "claude-orchestrator"

	// DefaultWorkflowTimeout is the default timeout for workflows.
	DefaultWorkflowTimeout = 24 * time.Hour

	// DefaultActivityTimeout is the default timeout for activities.
	DefaultActivityTimeout = 10 * time.Minute
)

// OrchestratorInput represents the input to the orchestrator workflow.
type OrchestratorInput struct {
	Task          domain.Task            `json:"task"`
	Config        OrchestratorConfig     `json:"config"`
	Context       map[string]interface{} `json:"context,omitempty"`
}

// OrchestratorConfig contains configuration for the orchestrator.
type OrchestratorConfig struct {
	MaxAgents       int                       `json:"max_agents"`
	MaxRetries      int                       `json:"max_retries"`
	TimeoutMinutes  int                       `json:"timeout_minutes"`
	AgentConfigs    map[domain.AgentType]domain.AgentConfig `json:"agent_configs,omitempty"`
	EnableReview    bool                      `json:"enable_review"`
	ParallelTasks   bool                      `json:"parallel_tasks"`
}

// OrchestratorOutput represents the output of the orchestrator workflow.
type OrchestratorOutput struct {
	Status        string                    `json:"status"`
	Results       []AgentWorkflowOutput     `json:"results"`
	Summary       string                    `json:"summary"`
	Metrics       OrchestratorMetrics       `json:"metrics"`
	Artifacts     []domain.Artifact         `json:"artifacts"`
	Error         string                    `json:"error,omitempty"`
}

// OrchestratorMetrics contains metrics for the orchestration.
type OrchestratorMetrics struct {
	TotalAgents      int           `json:"total_agents"`
	SuccessfulAgents int           `json:"successful_agents"`
	FailedAgents     int           `json:"failed_agents"`
	TotalTokens      int           `json:"total_tokens"`
	TotalDuration    time.Duration `json:"total_duration"`
}

// DefaultOrchestratorConfig returns the default configuration.
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		MaxAgents:      10,
		MaxRetries:     3,
		TimeoutMinutes: 60,
		EnableReview:   true,
		ParallelTasks:  true,
		AgentConfigs:   domain.DefaultAgentConfigs(),
	}
}

// OrchestratorWorkflow is the main orchestrator workflow (the Queen).
func OrchestratorWorkflow(ctx workflow.Context, input OrchestratorInput) (*OrchestratorOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting orchestrator workflow", "task_id", input.Task.ID, "task_title", input.Task.Title)

	startTime := workflow.Now(ctx)

	// Apply default config
	config := input.Config
	if config.MaxAgents == 0 {
		config = DefaultOrchestratorConfig()
	}
	if config.AgentConfigs == nil {
		config.AgentConfigs = domain.DefaultAgentConfigs()
	}

	// Set up activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    5 * time.Minute, // Increased for Claude Code CLI
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    int32(config.MaxRetries),
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	output := &OrchestratorOutput{
		Results:   []AgentWorkflowOutput{},
		Artifacts: []domain.Artifact{},
	}

	// Step 1: Decompose the task using the orchestrator agent
	logger.Info("Decomposing task into subtasks")

	var decomposition *activity.DecomposeTaskResult
	err := workflow.ExecuteActivity(ctx, "DecomposeTask", activity.DecomposeTaskRequest{
		Task:        input.Task,
		AgentConfig: config.AgentConfigs[domain.AgentTypeOrchestrator],
		MaxSubtasks: config.MaxAgents,
	}).Get(ctx, &decomposition)

	if err != nil {
		logger.Error("Failed to decompose task", "error", err)
		output.Status = "failed"
		output.Error = fmt.Sprintf("Failed to decompose task: %v", err)
		return output, err
	}

	logger.Info("Task decomposed", "subtask_count", len(decomposition.Subtasks), "strategy", decomposition.Strategy)

	// Convert subtask definitions to tasks
	subtasks := make([]domain.Task, len(decomposition.Subtasks))
	for i, def := range decomposition.Subtasks {
		subtasks[i] = domain.Task{
			ID:          fmt.Sprintf("%s-sub-%d", input.Task.ID, i),
			ParentID:    input.Task.ID,
			Type:        def.Type,
			Title:       def.Title,
			Description: def.Description,
			Status:      domain.TaskStatusPending,
			Priority:    def.Priority,
			AssignedTo:  def.AssignTo,
			Dependencies: def.Dependencies,
			Input:       def.Input,
			Context:     input.Task.Context,
			CreatedAt:   workflow.Now(ctx),
			MaxRetries:  config.MaxRetries,
		}
	}

	// Step 2: Execute subtasks
	var agentResults []AgentWorkflowOutput

	if config.ParallelTasks && len(subtasks) > 1 {
		// Execute independent tasks in parallel
		agentResults, err = executeTasksParallel(ctx, subtasks, config)
	} else {
		// Execute tasks sequentially
		agentResults, err = executeTasksSequential(ctx, subtasks, config)
	}

	if err != nil {
		logger.Error("Failed to execute subtasks", "error", err)
		output.Status = "failed"
		output.Error = fmt.Sprintf("Failed to execute subtasks: %v", err)
		return output, err
	}

	output.Results = agentResults

	// Step 3: Review results if enabled
	if config.EnableReview {
		logger.Info("Reviewing results")

		var reviewResult *AgentWorkflowOutput
		reviewInput := AgentWorkflowInput{
			Task: domain.Task{
				ID:          fmt.Sprintf("%s-review", input.Task.ID),
				ParentID:    input.Task.ID,
				Type:        "review",
				Title:       "Review agent outputs",
				Description: fmt.Sprintf("Review the outputs from %d agents and provide a summary", len(agentResults)),
			},
			AgentConfig: config.AgentConfigs[domain.AgentTypeReviewer],
			Context:     buildReviewContext(agentResults),
		}

		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID:         fmt.Sprintf("%s-review", workflow.GetInfo(ctx).WorkflowExecution.ID),
			TaskQueue:          TaskQueueName,
			WorkflowRunTimeout: time.Duration(config.TimeoutMinutes) * time.Minute,
		})

		err = workflow.ExecuteChildWorkflow(childCtx, ReviewerWorkflow, reviewInput).Get(ctx, &reviewResult)
		if err != nil {
			logger.Warn("Review failed, continuing without review", "error", err)
		} else {
			output.Results = append(output.Results, *reviewResult)
			output.Summary = reviewResult.Output
		}
	}

	// Step 4: Aggregate metrics and artifacts
	metrics := OrchestratorMetrics{
		TotalAgents:   len(agentResults),
		TotalDuration: workflow.Now(ctx).Sub(startTime),
	}

	for _, result := range agentResults {
		if result.Status == string(domain.AgentStatusCompleted) {
			metrics.SuccessfulAgents++
		} else if result.Status == string(domain.AgentStatusFailed) {
			metrics.FailedAgents++
		}
		metrics.TotalTokens += result.Metrics.TokensUsed
		output.Artifacts = append(output.Artifacts, result.Artifacts...)
	}

	output.Metrics = metrics

	if metrics.FailedAgents == 0 {
		output.Status = "completed"
	} else if metrics.SuccessfulAgents > 0 {
		output.Status = "partial"
	} else {
		output.Status = "failed"
	}

	logger.Info("Orchestrator workflow completed",
		"status", output.Status,
		"total_agents", metrics.TotalAgents,
		"successful", metrics.SuccessfulAgents,
		"failed", metrics.FailedAgents,
		"total_tokens", metrics.TotalTokens)

	return output, nil
}

// executeTasksParallel executes tasks in parallel where possible.
func executeTasksParallel(ctx workflow.Context, tasks []domain.Task, config OrchestratorConfig) ([]AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)

	// Build dependency graph
	taskByID := make(map[string]*domain.Task)
	for i := range tasks {
		taskByID[tasks[i].ID] = &tasks[i]
	}

	completed := make(map[string]bool)
	results := make([]AgentWorkflowOutput, 0, len(tasks))

	// Track pending futures
	type pendingTask struct {
		task   domain.Task
		future workflow.ChildWorkflowFuture
	}
	var pending []pendingTask

	for len(completed) < len(tasks) {
		// Find tasks ready to execute
		var ready []domain.Task
		for _, task := range tasks {
			if completed[task.ID] {
				continue
			}

			// Check if already pending
			isPending := false
			for _, p := range pending {
				if p.task.ID == task.ID {
					isPending = true
					break
				}
			}
			if isPending {
				continue
			}

			// Check dependencies
			depsComplete := true
			for _, depID := range task.Dependencies {
				if !completed[depID] {
					depsComplete = false
					break
				}
			}

			if depsComplete {
				ready = append(ready, task)
			}
		}

		// Start ready tasks
		for _, task := range ready {
			logger.Info("Starting agent for task", "task_id", task.ID, "agent_type", task.AssignedTo)

			agentConfig := config.AgentConfigs[task.AssignedTo]
			if agentConfig.ID == "" {
				agentConfig = domain.DefaultAgentConfigs()[task.AssignedTo]
			}

			input := AgentWorkflowInput{
				Task:        task,
				AgentConfig: agentConfig,
			}

			childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowID:         fmt.Sprintf("%s-agent-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, task.ID),
				TaskQueue:          TaskQueueName,
				WorkflowRunTimeout: time.Duration(config.TimeoutMinutes) * time.Minute,
			})

			var wf interface{}
			switch task.AssignedTo {
			case domain.AgentTypePlanner:
				wf = PlannerWorkflow
			case domain.AgentTypeResearcher:
				wf = ResearcherWorkflow
			case domain.AgentTypeCoder:
				wf = CoderWorkflow
			case domain.AgentTypeReviewer:
				wf = ReviewerWorkflow
			case domain.AgentTypeExecutor:
				wf = ExecutorWorkflow
			default:
				wf = AgentWorkflow
			}

			future := workflow.ExecuteChildWorkflow(childCtx, wf, input)
			pending = append(pending, pendingTask{task: task, future: future})
		}

		if len(pending) == 0 {
			// No tasks ready and none pending - check for circular dependencies
			break
		}

		// Wait for at least one task to complete
		selector := workflow.NewSelector(ctx)
		for i, p := range pending {
			idx := i
			pt := p
			selector.AddFuture(pt.future, func(f workflow.Future) {
				var result AgentWorkflowOutput
				if err := f.Get(ctx, &result); err != nil {
					logger.Error("Agent workflow failed", "task_id", pt.task.ID, "error", err)
					result = AgentWorkflowOutput{
						AgentID: pt.task.ID,
						Status:  string(domain.AgentStatusFailed),
						Error:   err.Error(),
					}
				}
				results = append(results, result)
				completed[pt.task.ID] = true

				// Remove from pending
				pending = append(pending[:idx], pending[idx+1:]...)
			})
		}
		selector.Select(ctx)
	}

	return results, nil
}

// executeTasksSequential executes tasks one at a time.
func executeTasksSequential(ctx workflow.Context, tasks []domain.Task, config OrchestratorConfig) ([]AgentWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	results := make([]AgentWorkflowOutput, 0, len(tasks))

	for _, task := range tasks {
		logger.Info("Executing task", "task_id", task.ID, "agent_type", task.AssignedTo)

		agentConfig := config.AgentConfigs[task.AssignedTo]
		if agentConfig.ID == "" {
			agentConfig = domain.DefaultAgentConfigs()[task.AssignedTo]
		}

		input := AgentWorkflowInput{
			Task:        task,
			AgentConfig: agentConfig,
		}

		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID:         fmt.Sprintf("%s-agent-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, task.ID),
			TaskQueue:          TaskQueueName,
			WorkflowRunTimeout: time.Duration(config.TimeoutMinutes) * time.Minute,
		})

		var wf interface{}
		switch task.AssignedTo {
		case domain.AgentTypePlanner:
			wf = PlannerWorkflow
		case domain.AgentTypeResearcher:
			wf = ResearcherWorkflow
		case domain.AgentTypeCoder:
			wf = CoderWorkflow
		case domain.AgentTypeReviewer:
			wf = ReviewerWorkflow
		case domain.AgentTypeExecutor:
			wf = ExecutorWorkflow
		default:
			wf = AgentWorkflow
		}

		var result AgentWorkflowOutput
		if err := workflow.ExecuteChildWorkflow(childCtx, wf, input).Get(ctx, &result); err != nil {
			logger.Error("Agent workflow failed", "task_id", task.ID, "error", err)
			result = AgentWorkflowOutput{
				AgentID: task.ID,
				Status:  string(domain.AgentStatusFailed),
				Error:   err.Error(),
			}
		}
		results = append(results, result)
	}

	return results, nil
}

// buildReviewContext creates context for the review step.
func buildReviewContext(results []AgentWorkflowOutput) map[string]interface{} {
	summaries := make([]map[string]interface{}, len(results))
	for i, r := range results {
		summaries[i] = map[string]interface{}{
			"agent_id": r.AgentID,
			"status":   r.Status,
			"output":   r.Output,
			"error":    r.Error,
		}
	}
	return map[string]interface{}{
		"agent_results": summaries,
		"result_count":  len(results),
	}
}
