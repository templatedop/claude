// Package main provides the CLI for the Claude orchestrator.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/domain"
	"github.com/anthropics/claude-orchestrator/internal/tui"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var (
	temporalAddr string
	temporalNS   string
	taskQueue    string
	outputFormat string
	configPath   string
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
	rootCmd.PersistentFlags().StringVar(&configPath, "config", getEnv("CONFIG_PATH", ""), "Path to configuration file (YAML or JSON)")
	rootCmd.PersistentFlags().StringVar(&temporalAddr, "temporal-addr", getEnv("TEMPORAL_ADDRESS", "localhost:7233"), "Temporal server address")
	rootCmd.PersistentFlags().StringVar(&temporalNS, "namespace", getEnv("TEMPORAL_NAMESPACE", "default"), "Temporal namespace")
	rootCmd.PersistentFlags().StringVar(&taskQueue, "task-queue", getEnv("TASK_QUEUE", workflow.TaskQueueName), "Task queue name")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")

	// Add commands
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cancelCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(tuiCmd())

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

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or manage configuration",
		Long: `Display current configuration settings including:
- Claude provider (api or claude_code)
- Temporal connection settings
- Environment variable status`,
	}

	cmd.AddCommand(configShowCmd())
	cmd.AddCommand(configSetCmd())

	return cmd
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := getConfigInfo()

			if outputFormat == "json" {
				return outputJSON(config)
			}

			fmt.Println("Claude Orchestrator Configuration")
			fmt.Println(repeatChar('=', 40))
			fmt.Println()

			fmt.Println("Claude Provider:")
			fmt.Printf("  CLAUDE_PROVIDER:    %s\n", valueOrDefault(config["claude_provider"], "(not set, defaults to auto-detect)"))
			fmt.Printf("  ANTHROPIC_API_KEY:  %s\n", maskValue(config["api_key_status"]))
			fmt.Println()

			fmt.Println("Temporal Settings:")
			fmt.Printf("  Address:   %s\n", config["temporal_address"])
			fmt.Printf("  Namespace: %s\n", config["temporal_namespace"])
			fmt.Printf("  TaskQueue: %s\n", config["task_queue"])
			fmt.Println()

			fmt.Println("Provider Behavior:")
			if config["api_key_status"] == "set" {
				if config["claude_provider"] == "claude_code" {
					fmt.Println("  → Using Claude Code CLI (subscription-based)")
				} else {
					fmt.Println("  → Using Claude API (credits-based)")
				}
			} else {
				if config["claude_provider"] == "api" {
					fmt.Println("  ⚠ API provider selected but ANTHROPIC_API_KEY not set")
				} else {
					fmt.Println("  → Using Claude Code CLI (subscription-based)")
				}
			}
			fmt.Println()

			fmt.Println("To change provider:")
			fmt.Println("  export CLAUDE_PROVIDER=api          # Use API (requires ANTHROPIC_API_KEY)")
			fmt.Println("  export CLAUDE_PROVIDER=claude_code  # Use Claude Code CLI")

			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [provider]",
		Short: "Show how to set configuration",
		Long: `Show instructions for setting the Claude provider.

Valid providers:
  api         - Use Claude API (requires ANTHROPIC_API_KEY)
  claude_code - Use Claude Code CLI (requires 'claude login')`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println("Usage: claude-orchestrator config set <provider>")
				fmt.Println()
				fmt.Println("Valid providers:")
				fmt.Println("  api         - Use Claude API (credits-based)")
				fmt.Println("  claude_code - Use Claude Code CLI (subscription-based)")
				fmt.Println()
				fmt.Println("Example:")
				fmt.Println("  # Set provider to Claude Code")
				fmt.Println("  export CLAUDE_PROVIDER=claude_code")
				fmt.Println()
				fmt.Println("  # Set provider to API")
				fmt.Println("  export CLAUDE_PROVIDER=api")
				fmt.Println("  export ANTHROPIC_API_KEY=your-api-key")
				return nil
			}

			provider := strings.ToLower(args[0])
			switch provider {
			case "api":
				fmt.Println("To use Claude API provider:")
				fmt.Println()
				fmt.Println("  export CLAUDE_PROVIDER=api")
				fmt.Println("  export ANTHROPIC_API_KEY=your-api-key")
				fmt.Println()
				fmt.Println("Then restart the worker.")
			case "claude_code", "claudecode", "cli":
				fmt.Println("To use Claude Code CLI provider:")
				fmt.Println()
				fmt.Println("  1. Install Claude Code: npm install -g @anthropic-ai/claude-code")
				fmt.Println("  2. Authenticate: claude login")
				fmt.Println("  3. Set provider: export CLAUDE_PROVIDER=claude_code")
				fmt.Println()
				fmt.Println("Then restart the worker.")
			default:
				return fmt.Errorf("unknown provider: %s (valid: api, claude_code)", provider)
			}

			return nil
		},
	}
}

func getConfigInfo() map[string]string {
	provider := os.Getenv("CLAUDE_PROVIDER")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")

	apiKeyStatus := "not set"
	if apiKey != "" {
		apiKeyStatus = "set"
	}

	return map[string]string{
		"claude_provider":    provider,
		"api_key_status":     apiKeyStatus,
		"temporal_address":   temporalAddr,
		"temporal_namespace": temporalNS,
		"task_queue":         taskQueue,
	}
}

func valueOrDefault(value, defaultVal string) string {
	if value == "" {
		return defaultVal
	}
	return value
}

func maskValue(status string) string {
	if status == "set" {
		return "****** (set)"
	}
	return "(not set)"
}

func outputJSON(data interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func tuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive terminal UI",
		Long: `Launch an interactive Bubble Tea-based terminal UI for managing
orchestration workflows. The TUI provides:

- Real-time workflow status monitoring
- Task creation and management
- Agent activity visualization
- Log viewing and filtering`,
		Example: `  # Launch TUI with default config
  claude-orchestrator tui

  # Launch TUI with Temporal settings
  claude-orchestrator tui --temporal-addr localhost:7233`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create and run the TUI
			model := tui.NewModel()
			p := tea.NewProgram(model, tea.WithAltScreen())

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}

			return nil
		},
	}

	return cmd
}
