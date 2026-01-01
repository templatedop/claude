// Package main provides the Temporal worker for the Claude orchestrator.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/anthropics/claude-orchestrator/internal/activity"
	"github.com/anthropics/claude-orchestrator/internal/config"
	"github.com/anthropics/claude-orchestrator/internal/memory"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.LoadConfig(configPath)
	if err != nil && configPath != "" {
		log.Printf("Warning: Failed to load config from %s: %v", configPath, err)
		cfg = config.DefaultConfig()
	}

	// Apply environment overrides for backwards compatibility
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		cfg.Claude.APIKey = apiKey
	}
	workingDir := getEnv("WORKING_DIR", ".")

	if cfg.Claude.APIKey == "" {
		log.Println("Warning: ANTHROPIC_API_KEY not set. Claude activities will fail.")
	}

	// Log feature status
	log.Println("Feature Status:")
	log.Printf("  - Document Analysis: %v", cfg.Features.DocumentAnalysis.Enabled)
	log.Printf("  - Requirements Tracking: %v", cfg.Features.RequirementsTracking.Enabled)
	log.Printf("  - Framework Learning: %v", cfg.Features.FrameworkLearning.Enabled)
	log.Printf("  - Code Review: %v", cfg.Features.CodeReview)
	log.Printf("  - Parallel Execution: %v", cfg.Features.ParallelExecution)

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.Address,
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	// Create memory store
	var memoryStore memory.Store
	switch cfg.Memory.Type {
	case "postgres":
		pgCfg := memory.PostgresConfig{
			Host:     cfg.Memory.Postgres.Host,
			Port:     cfg.Memory.Postgres.Port,
			Database: cfg.Memory.Postgres.Database,
			User:     cfg.Memory.Postgres.User,
			Password: cfg.Memory.Postgres.Password,
			SSLMode:  cfg.Memory.Postgres.SSLMode,
		}
		store, err := memory.NewPostgresStore(pgCfg)
		if err != nil {
			log.Printf("Failed to create PostgreSQL store, falling back to in-memory: %v", err)
			memoryStore = memory.NewInMemoryStore()
		} else {
			memoryStore = store
		}
	default:
		memoryStore = memory.NewInMemoryStore()
	}
	defer memoryStore.Close()

	// Create core activities
	var claudeActivities *activity.ClaudeActivities
	if cfg.Claude.APIKey != "" {
		claudeActivities, err = activity.NewClaudeActivities(cfg.Claude.APIKey)
		if err != nil {
			log.Fatalf("Failed to create Claude activities: %v", err)
		}
	}

	fsActivities := activity.NewFileSystemActivities(workingDir, []string{workingDir})
	gitActivities := activity.NewGitActivities(workingDir)
	memoryActivities := activity.NewMemoryActivities(memoryStore)

	// Create feature-specific activities
	var documentActivities *activity.DocumentActivities
	var requirementsActivities *activity.RequirementsActivities
	var frameworkActivities *activity.FrameworkActivities

	if cfg.Features.DocumentAnalysis.Enabled && cfg.Claude.APIKey != "" {
		documentActivities, err = activity.NewDocumentActivities(cfg.Claude.APIKey)
		if err != nil {
			log.Printf("Warning: Failed to create document activities: %v", err)
		}
	}

	if cfg.Features.RequirementsTracking.Enabled {
		requirementsActivities = activity.NewRequirementsActivities(memoryStore)
	}

	if cfg.Features.FrameworkLearning.Enabled && cfg.Claude.APIKey != "" {
		frameworkActivities, err = activity.NewFrameworkActivities(cfg.Claude.APIKey, memoryStore)
		if err != nil {
			log.Printf("Warning: Failed to create framework activities: %v", err)
		}
	}

	// Create worker
	w := worker.New(c, cfg.Temporal.TaskQueue, worker.Options{
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

	// Register core activities
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

	// Register feature-specific activities
	if documentActivities != nil {
		w.RegisterActivity(documentActivities.ReadDocument)
		w.RegisterActivity(documentActivities.AnalyzeRequirements)
		log.Println("Registered document analysis activities")
	}

	if requirementsActivities != nil {
		w.RegisterActivity(requirementsActivities.StoreRequirement)
		w.RegisterActivity(requirementsActivities.GetRequirement)
		w.RegisterActivity(requirementsActivities.ListRequirements)
		w.RegisterActivity(requirementsActivities.UpdateRequirementStatus)
		w.RegisterActivity(requirementsActivities.LinkImplementation)
		w.RegisterActivity(requirementsActivities.GetRequirementsSummary)
		log.Println("Registered requirements tracking activities")
	}

	if frameworkActivities != nil {
		w.RegisterActivity(frameworkActivities.LearnFramework)
		w.RegisterActivity(frameworkActivities.GetFrameworkKnowledge)
		w.RegisterActivity(frameworkActivities.GenerateFrameworkPrompt)
		w.RegisterActivity(frameworkActivities.ListFrameworks)
		log.Println("Registered framework learning activities")
	}

	// Start worker in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatalf("Worker failed: %v", err)
		}
	}()

	log.Printf("Worker started on task queue: %s", cfg.Temporal.TaskQueue)
	log.Printf("Temporal address: %s", cfg.Temporal.Address)
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
