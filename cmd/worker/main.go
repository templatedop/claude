// Package main provides the Temporal worker for the Claude orchestrator.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/anthropics/claude-orchestrator/internal/activity"
	"github.com/anthropics/claude-orchestrator/internal/memory"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	// Get configuration from environment
	temporalAddr := getEnv("TEMPORAL_ADDRESS", "localhost:7233")
	temporalNS := getEnv("TEMPORAL_NAMESPACE", "default")
	taskQueue := getEnv("TASK_QUEUE", workflow.TaskQueueName)
	claudeAPIKey := getEnv("ANTHROPIC_API_KEY", "")
	workingDir := getEnv("WORKING_DIR", ".")

	if claudeAPIKey == "" {
		log.Println("Warning: ANTHROPIC_API_KEY not set. Claude activities will fail.")
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  temporalAddr,
		Namespace: temporalNS,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	// Create memory store
	memoryStore := memory.NewInMemoryStore()
	defer memoryStore.Close()

	// Create activities
	var claudeActivities *activity.ClaudeActivities
	if claudeAPIKey != "" {
		claudeActivities, err = activity.NewClaudeActivities(claudeAPIKey)
		if err != nil {
			log.Fatalf("Failed to create Claude activities: %v", err)
		}
	}

	fsActivities := activity.NewFileSystemActivities(workingDir, []string{workingDir})
	gitActivities := activity.NewGitActivities(workingDir)
	memoryActivities := activity.NewMemoryActivities(memoryStore)

	// Create worker
	w := worker.New(c, taskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     10,
		MaxConcurrentWorkflowTaskExecutionSize: 10,
	})

	// Register workflows
	w.RegisterWorkflow(workflow.OrchestratorWorkflow)
	w.RegisterWorkflow(workflow.AgentWorkflow)
	w.RegisterWorkflow(workflow.PlannerWorkflow)
	w.RegisterWorkflow(workflow.ResearcherWorkflow)
	w.RegisterWorkflow(workflow.CoderWorkflow)
	w.RegisterWorkflow(workflow.ReviewerWorkflow)
	w.RegisterWorkflow(workflow.ExecutorWorkflow)

	// Register activities
	if claudeActivities != nil {
		w.RegisterActivity(claudeActivities.Complete)
		w.RegisterActivity(claudeActivities.DecomposeTask)
		w.RegisterActivity(claudeActivities.AnalyzeCode)
		w.RegisterActivity(claudeActivities.GenerateCode)
	}

	w.RegisterActivity(fsActivities.ReadFile)
	w.RegisterActivity(fsActivities.WriteFile)
	w.RegisterActivity(fsActivities.DeleteFile)
	w.RegisterActivity(fsActivities.ListFiles)
	w.RegisterActivity(fsActivities.CopyFile)
	w.RegisterActivity(fsActivities.CreateDirectory)

	w.RegisterActivity(gitActivities.Status)
	w.RegisterActivity(gitActivities.Commit)
	w.RegisterActivity(gitActivities.Diff)
	w.RegisterActivity(gitActivities.Branch)
	w.RegisterActivity(gitActivities.Log)
	w.RegisterActivity(gitActivities.Pull)
	w.RegisterActivity(gitActivities.Push)
	w.RegisterActivity(gitActivities.Add)

	w.RegisterActivity(memoryActivities.Store)
	w.RegisterActivity(memoryActivities.Get)
	w.RegisterActivity(memoryActivities.Delete)
	w.RegisterActivity(memoryActivities.Query)
	w.RegisterActivity(memoryActivities.VectorSearch)
	w.RegisterActivity(memoryActivities.ListKeys)
	w.RegisterActivity(memoryActivities.Clear)

	// Start worker in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatalf("Worker failed: %v", err)
		}
	}()

	log.Printf("Worker started on task queue: %s", taskQueue)
	log.Printf("Temporal address: %s", temporalAddr)
	log.Printf("Working directory: %s", workingDir)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	case <-ctx.Done():
	}

	log.Println("Worker stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
