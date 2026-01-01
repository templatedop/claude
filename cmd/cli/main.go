// Package main provides the CLI for the Claude orchestrator.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/domain"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var (
	temporalAddr string
	temporalNS   string
	taskQueue    string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "claude-orchestrator",
		Short: "Claude Code Orchestrator - Coordinate AI agents for software development",
		Long: `Claude Orchestrator is a Temporal-based system for coordinating
multiple Claude AI agents to accomplish complex software development tasks.

It uses a "Queen" orchestrator pattern to decompose tasks, delegate to
specialized agents (Planner, Researcher, Coder, Reviewer, Executor),
and aggregate results.`,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&temporalAddr, "temporal-addr", getEnv("TEMPORAL_ADDRESS", "localhost:7233"), "Temporal server address")
	rootCmd.PersistentFlags().StringVar(&temporalNS, "namespace", getEnv("TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	rootCmd.PersistentFlags().StringVar(&taskQueue, "task-queue", getEnv("TASK_QUEUE", workflow.TaskQueueName), "Task queue name")

	// Add commands
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cancelCmd())
	rootCmd.AddCommand(listCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	var (
		title       string
		description string
		maxAgents   int
		timeout     int
		parallel    bool
		review      bool
		wait        bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a new orchestration task",
		Long: `Start a new orchestration workflow with the specified task.
The orchestrator will decompose the task and delegate to specialized agents.`,
		Example: `  # Run a simple task
  claude-orchestrator run --title "Build REST API" --description "Create a REST API with user authentication"

  # Run with specific options
  claude-orchestrator run --title "Refactor auth" --description "Refactor authentication module" --max-agents 5 --parallel

  # Run and wait for completion
  claude-orchestrator run --title "Fix bug" --description "Fix the login bug" --wait`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if description == "" {
				description = title
			}

			c, err := createClient()
			if err != nil {
				return err
			}
			defer c.Close()

			// Create task
			taskID := uuid.New().String()
			task := domain.Task{
				ID:          taskID,
				Title:       title,
				Description: description,
				Status:      domain.TaskStatusPending,
				Priority:    domain.TaskPriorityNormal,
				CreatedAt:   time.Now(),
				Context: domain.TaskContext{
					WorkingDir: ".",
					TimeoutSec: timeout * 60,
				},
			}

			// Create input
			input := workflow.OrchestratorInput{
				Task: task,
				Config: workflow.OrchestratorConfig{
					MaxAgents:      maxAgents,
					MaxRetries:     3,
					TimeoutMinutes: timeout,
					EnableReview:   review,
					ParallelTasks:  parallel,
				},
			}

			// Start workflow
			workflowID := fmt.Sprintf("orchestrator-%s", taskID)
			options := client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: taskQueue,
			}

			we, err := c.ExecuteWorkflow(context.Background(), options, workflow.OrchestratorWorkflow, input)
			if err != nil {
				return fmt.Errorf("failed to start workflow: %w", err)
			}

			fmt.Printf("Started orchestration workflow\n")
			fmt.Printf("  Workflow ID: %s\n", we.GetID())
			fmt.Printf("  Run ID: %s\n", we.GetRunID())
			fmt.Printf("  Task: %s\n", title)

			if wait {
				fmt.Println("\nWaiting for completion...")

				var result workflow.OrchestratorOutput
				if err := we.Get(context.Background(), &result); err != nil {
					return fmt.Errorf("workflow failed: %w", err)
				}

				fmt.Printf("\nWorkflow completed!\n")
				fmt.Printf("  Status: %s\n", result.Status)
				fmt.Printf("  Agents: %d total, %d successful, %d failed\n",
					result.Metrics.TotalAgents,
					result.Metrics.SuccessfulAgents,
					result.Metrics.FailedAgents)
				fmt.Printf("  Tokens used: %d\n", result.Metrics.TotalTokens)
				fmt.Printf("  Duration: %s\n", result.Metrics.TotalDuration)

				if result.Summary != "" {
					fmt.Printf("\nSummary:\n%s\n", result.Summary)
				}

				if result.Error != "" {
					fmt.Printf("\nError: %s\n", result.Error)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "Task title (required)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Task description")
	cmd.Flags().IntVarP(&maxAgents, "max-agents", "a", 10, "Maximum number of agents")
	cmd.Flags().IntVar(&timeout, "timeout", 60, "Workflow timeout in minutes")
	cmd.Flags().BoolVar(&parallel, "parallel", true, "Execute independent tasks in parallel")
	cmd.Flags().BoolVar(&review, "review", true, "Enable code review agent")
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "Wait for workflow completion")

	return cmd
}

func statusCmd() *cobra.Command {
	var detailed bool

	cmd := &cobra.Command{
		Use:   "status [workflow-id]",
		Short: "Get the status of an orchestration workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			c, err := createClient()
			if err != nil {
				return err
			}
			defer c.Close()

			// Describe workflow
			resp, err := c.DescribeWorkflowExecution(context.Background(), workflowID, "")
			if err != nil {
				return fmt.Errorf("failed to describe workflow: %w", err)
			}

			info := resp.WorkflowExecutionInfo
			fmt.Printf("Workflow: %s\n", info.Execution.WorkflowId)
			fmt.Printf("Run ID: %s\n", info.Execution.RunId)
			fmt.Printf("Status: %s\n", info.Status.String())
			fmt.Printf("Started: %s\n", info.StartTime.AsTime().Format(time.RFC3339))

			if info.CloseTime != nil {
				fmt.Printf("Completed: %s\n", info.CloseTime.AsTime().Format(time.RFC3339))
				duration := info.CloseTime.AsTime().Sub(info.StartTime.AsTime())
				fmt.Printf("Duration: %s\n", duration)
			}

			if detailed {
				// Get workflow result if completed
				if info.Status.String() == "Completed" {
					run := c.GetWorkflow(context.Background(), workflowID, "")
					var result workflow.OrchestratorOutput
					if err := run.Get(context.Background(), &result); err == nil {
						fmt.Printf("\nResults:\n")
						resultJSON, _ := json.MarshalIndent(result, "  ", "  ")
						fmt.Printf("  %s\n", string(resultJSON))
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&detailed, "detailed", "d", false, "Show detailed output")

	return cmd
}

func cancelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel [workflow-id]",
		Short: "Cancel a running orchestration workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := args[0]

			c, err := createClient()
			if err != nil {
				return err
			}
			defer c.Close()

			err = c.CancelWorkflow(context.Background(), workflowID, "")
			if err != nil {
				return fmt.Errorf("failed to cancel workflow: %w", err)
			}

			fmt.Printf("Cancelled workflow: %s\n", workflowID)
			return nil
		},
	}

	return cmd
}

func listCmd() *cobra.Command {
	var (
		limit  int
		status string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List orchestration workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := createClient()
			if err != nil {
				return err
			}
			defer c.Close()

			query := fmt.Sprintf("WorkflowType = '%s'", "OrchestratorWorkflow")
			if status != "" {
				query += fmt.Sprintf(" AND ExecutionStatus = '%s'", status)
			}

			resp, err := c.ListWorkflow(context.Background(), &workflowservice.ListWorkflowExecutionsRequest{
				Namespace: temporalNS,
				Query:     query,
				PageSize:  int32(limit),
			})
			if err != nil {
				return fmt.Errorf("failed to list workflows: %w", err)
			}

			if len(resp.Executions) == 0 {
				fmt.Println("No workflows found")
				return nil
			}

			fmt.Printf("%-45s %-12s %-25s\n", "WORKFLOW ID", "STATUS", "STARTED")
			fmt.Println(repeatChar('-', 85))

			for _, exec := range resp.Executions {
				fmt.Printf("%-45s %-12s %-25s\n",
					truncate(exec.Execution.WorkflowId, 45),
					exec.Status.String(),
					exec.StartTime.AsTime().Format("2006-01-02 15:04:05"),
				)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Maximum number of workflows to list")
	cmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status (Running, Completed, Failed, Cancelled)")

	return cmd
}

func createClient() (client.Client, error) {
	return client.Dial(client.Options{
		HostPort:  temporalAddr,
		Namespace: temporalNS,
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func repeatChar(c rune, n int) string {
	result := make([]rune, n)
	for i := range result {
		result[i] = c
	}
	return string(result)
}
