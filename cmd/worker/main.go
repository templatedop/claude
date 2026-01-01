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
	"github.com/anthropics/claude-orchestrator/internal/rag"
	"github.com/anthropics/claude-orchestrator/internal/storage"
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
	log.Printf("  - Cloud Storage: %v (%s)", cfg.Storage.Enabled, cfg.Storage.Type)
	log.Printf("  - RAG/Embeddings: %v (%s)", cfg.RAG.Enabled, cfg.RAG.Embedding.Provider)

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

	// Create storage activities
	var storageActivities *activity.StorageActivities
	if cfg.Storage.Enabled {
		var fileStore storage.Store
		switch cfg.Storage.Type {
		case "s3", "minio":
			s3Cfg := storage.S3Config{
				Region:          cfg.Storage.S3.Region,
				Endpoint:        cfg.Storage.S3.Endpoint,
				AccessKeyID:     cfg.Storage.S3.AccessKeyID,
				SecretAccessKey: cfg.Storage.S3.SecretAccessKey,
				UsePathStyle:    cfg.Storage.S3.UsePathStyle,
			}
			s3Store, err := storage.NewS3Store(s3Cfg)
			if err != nil {
				log.Printf("Warning: Failed to create S3 store, falling back to local: %v", err)
				localCfg := storage.LocalConfig{
					BasePath:    cfg.Storage.Local.BasePath,
					MaxFileSize: cfg.Storage.Local.MaxFileSize,
				}
				fileStore, _ = storage.NewLocalStore(localCfg)
			} else {
				fileStore = s3Store
			}
		default: // local
			localCfg := storage.LocalConfig{
				BasePath:    cfg.Storage.Local.BasePath,
				BaseURL:     cfg.Storage.Local.BaseURL,
				MaxFileSize: cfg.Storage.Local.MaxFileSize,
			}
			localStore, err := storage.NewLocalStore(localCfg)
			if err != nil {
				log.Printf("Warning: Failed to create local storage: %v", err)
			} else {
				fileStore = localStore
			}
		}
		if fileStore != nil {
			storageActivities = activity.NewStorageActivities(fileStore)
		}
	}

	// Create RAG activities
	var ragActivities *activity.RAGActivities
	if cfg.RAG.Enabled {
		embeddingConfig := rag.EmbeddingConfig{
			Provider: rag.EmbeddingProvider(cfg.RAG.Embedding.Provider),
			Model:    rag.EmbeddingModel(cfg.RAG.Embedding.Model),
			APIKey:   cfg.RAG.Embedding.APIKey,
			Endpoint: cfg.RAG.Embedding.Endpoint,
		}
		chunkOpts := rag.ChunkOptions{
			Strategy:     rag.ChunkingStrategy(cfg.RAG.Chunking.Strategy),
			ChunkSize:    cfg.RAG.Chunking.ChunkSize,
			ChunkOverlap: cfg.RAG.Chunking.ChunkOverlap,
			MinChunkSize: cfg.RAG.Chunking.MinChunkSize,
			MaxChunkSize: cfg.RAG.Chunking.MaxChunkSize,
		}
		ragActivities = activity.NewRAGActivities(memoryStore, embeddingConfig, chunkOpts)
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

	// Register storage activities
	if storageActivities != nil {
		w.RegisterActivity(storageActivities.Upload)
		w.RegisterActivity(storageActivities.Download)
		w.RegisterActivity(storageActivities.DeleteFile)
		w.RegisterActivity(storageActivities.GetMetadata)
		w.RegisterActivity(storageActivities.GenerateUploadURL)
		w.RegisterActivity(storageActivities.GenerateDownloadURL)
		w.RegisterActivity(storageActivities.ListFiles)
		w.RegisterActivity(storageActivities.CopyFile)
		w.RegisterActivity(storageActivities.CreateBucket)
		w.RegisterActivity(storageActivities.ListBuckets)
		w.RegisterActivity(storageActivities.FileExists)
		log.Printf("Registered cloud storage activities (%s)", cfg.Storage.Type)
	}

	// Register RAG activities
	if ragActivities != nil {
		w.RegisterActivity(ragActivities.IndexDocument)
		w.RegisterActivity(ragActivities.Search)
		w.RegisterActivity(ragActivities.DeleteDocument)
		w.RegisterActivity(ragActivities.GenerateRAGPrompt)
		w.RegisterActivity(ragActivities.ListDocuments)
		log.Printf("Registered RAG activities (%s embeddings)", cfg.RAG.Embedding.Provider)
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
