// Package main provides the Temporal worker for the Claude orchestrator.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/activity"
	"github.com/anthropics/claude-orchestrator/internal/config"
	"github.com/anthropics/claude-orchestrator/internal/logging"
	"github.com/anthropics/claude-orchestrator/internal/memory"
	"github.com/anthropics/claude-orchestrator/internal/metrics"
	"github.com/anthropics/claude-orchestrator/internal/rag"
	"github.com/anthropics/claude-orchestrator/internal/storage"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	"github.com/anthropics/claude-orchestrator/pkg/claude"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.LoadConfig(configPath)
	if err != nil && configPath != "" {
		// Use default config and continue
		cfg = config.DefaultConfig()
	}

	// Initialize logger
	logCfg := logging.Config{
		Level:         logging.Level(cfg.Logging.Level),
		Format:        logging.Format(cfg.Logging.Format),
		Output:        cfg.Logging.Output,
		AddCaller:     cfg.Logging.AddCaller,
		AddStacktrace: cfg.Logging.AddStacktrace,
		Development:   cfg.Logging.Development,
	}
	if cfg.Logging.Sampling != nil {
		logCfg.SamplingConfig = &logging.SamplingConfig{
			Enabled:    cfg.Logging.Sampling.Enabled,
			Initial:    cfg.Logging.Sampling.Initial,
			Thereafter: cfg.Logging.Sampling.Thereafter,
		}
	}

	logger, err := logging.NewLogger(logCfg)
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()
	logging.SetGlobal(logger)

	log := logger.Named("worker")

	// Initialize metrics
	metricsCfg := metrics.Config{
		Enabled:              cfg.Metrics.Enabled,
		Address:              cfg.Metrics.Address,
		Path:                 cfg.Metrics.Path,
		Namespace:            cfg.Metrics.Namespace,
		EnableGoMetrics:      cfg.Metrics.EnableGoMetrics,
		EnableProcessMetrics: cfg.Metrics.EnableProcessMetrics,
		Buckets:              cfg.Metrics.Buckets,
	}
	m := metrics.New(metricsCfg)
	metrics.SetGlobal(m)

	// Set application info
	m.SetInfo(version, runtime.Version(), buildTime)

	// Start metrics server in background if enabled
	if cfg.Metrics.Enabled {
		go func() {
			log.Info("Starting metrics server",
				logging.String("address", cfg.Metrics.Address),
				logging.String("path", cfg.Metrics.Path),
			)
			if err := m.StartServer(); err != nil {
				log.Error("Metrics server failed", logging.Error(err))
			}
		}()
	}

	// Apply environment overrides for backwards compatibility
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		cfg.Claude.APIKey = apiKey
	}
	workingDir := getEnv("WORKING_DIR", cfg.Claude.WorkingDir)

	// Determine Claude provider
	provider := cfg.Claude.Provider
	if provider == "" {
		// Auto-detect: use claude_code if no API key, otherwise use api
		if cfg.Claude.APIKey == "" {
			provider = "claude_code"
		} else {
			provider = "api"
		}
		cfg.Claude.Provider = provider
	}

	// Validate provider requirements
	switch provider {
	case "api":
		if cfg.Claude.APIKey == "" {
			log.Warn("ANTHROPIC_API_KEY not set but using 'api' provider. Claude activities will fail.")
		}
	case "claude_code":
		log.Info("Using Claude Code provider (subscription-based)")
	default:
		log.Warn("Unknown Claude provider, defaulting to 'api'",
			logging.String("provider", provider),
		)
		provider = "api"
	}

	// Log feature status
	log.Info("Feature Status",
		logging.String("claude_provider", provider),
		logging.Bool("document_analysis", cfg.Features.DocumentAnalysis.Enabled),
		logging.Bool("requirements_tracking", cfg.Features.RequirementsTracking.Enabled),
		logging.Bool("framework_learning", cfg.Features.FrameworkLearning.Enabled),
		logging.Bool("code_review", cfg.Features.CodeReview),
		logging.Bool("parallel_execution", cfg.Features.ParallelExecution),
		logging.Bool("cloud_storage", cfg.Storage.Enabled),
		logging.String("storage_type", cfg.Storage.Type),
		logging.Bool("rag", cfg.RAG.Enabled),
		logging.String("embedding_provider", cfg.RAG.Embedding.Provider),
	)

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.Address,
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		log.Fatal("Failed to create Temporal client", logging.Error(err))
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
			log.Warn("Failed to create PostgreSQL store, falling back to in-memory",
				logging.Error(err),
			)
			memoryStore = memory.NewInMemoryStore()
		} else {
			memoryStore = store
		}
	default:
		memoryStore = memory.NewInMemoryStore()
	}
	defer memoryStore.Close()

	// Create core activities with the configured provider
	var claudeActivities *activity.ClaudeActivities
	switch provider {
	case "claude_code":
		// Use Claude Code client (subscription-based)
		claudeActivities, err = activity.NewClaudeCodeActivities(
			claude.WithClaudeCodeWorkingDir(workingDir),
			claude.WithClaudeCodeModel(cfg.Claude.Model),
			claude.WithClaudeCodeTools(cfg.Claude.AllowedTools),
		)
		if err != nil {
			log.Fatal("Failed to create Claude Code activities", logging.Error(err))
		}
		log.Info("Created Claude Code activities",
			logging.String("working_dir", workingDir),
			logging.String("model", cfg.Claude.Model),
		)
	case "api":
		// Use API client (credits-based)
		if cfg.Claude.APIKey != "" {
			claudeActivities, err = activity.NewClaudeActivities(cfg.Claude.APIKey)
			if err != nil {
				log.Fatal("Failed to create Claude API activities", logging.Error(err))
			}
			log.Info("Created Claude API activities",
				logging.String("model", cfg.Claude.Model),
			)
		}
	}

	// Ensure cleanup of Claude client resources
	if claudeActivities != nil {
		defer claudeActivities.Close()
	}

	fsActivities := activity.NewFileSystemActivities(workingDir, []string{workingDir})
	gitActivities := activity.NewGitActivities(workingDir)
	memoryActivities := activity.NewMemoryActivities(memoryStore)

	// Create feature-specific activities
	var documentActivities *activity.DocumentActivities
	var requirementsActivities *activity.RequirementsActivities
	var frameworkActivities *activity.FrameworkActivities

	// Document and framework activities require a Claude client
	claudeClientAvailable := claudeActivities != nil

	if cfg.Features.DocumentAnalysis.Enabled && claudeClientAvailable {
		// Note: DocumentActivities currently only supports API client
		// Future: refactor to use ClaudeClient interface
		if cfg.Claude.APIKey != "" {
			documentActivities, err = activity.NewDocumentActivities(cfg.Claude.APIKey)
			if err != nil {
				log.Warn("Failed to create document activities", logging.Error(err))
			}
		} else {
			log.Warn("Document analysis requires API client (claude_code not yet supported)")
		}
	}

	if cfg.Features.RequirementsTracking.Enabled {
		requirementsActivities = activity.NewRequirementsActivities(memoryStore)
	}

	if cfg.Features.FrameworkLearning.Enabled && claudeClientAvailable {
		// Note: FrameworkActivities currently only supports API client
		// Future: refactor to use ClaudeClient interface
		if cfg.Claude.APIKey != "" {
			frameworkActivities, err = activity.NewFrameworkActivities(cfg.Claude.APIKey, memoryStore)
			if err != nil {
				log.Warn("Failed to create framework activities", logging.Error(err))
			}
		} else {
			log.Warn("Framework learning requires API client (claude_code not yet supported)")
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
				log.Warn("Failed to create S3 store, falling back to local", logging.Error(err))
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
				log.Warn("Failed to create local storage", logging.Error(err))
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

	// Create worker with interceptors for metrics
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
		log.Info("Registered document analysis activities")
	}

	if requirementsActivities != nil {
		w.RegisterActivity(requirementsActivities.StoreRequirement)
		w.RegisterActivity(requirementsActivities.GetRequirement)
		w.RegisterActivity(requirementsActivities.ListRequirements)
		w.RegisterActivity(requirementsActivities.UpdateRequirementStatus)
		w.RegisterActivity(requirementsActivities.LinkImplementation)
		w.RegisterActivity(requirementsActivities.GetRequirementsSummary)
		log.Info("Registered requirements tracking activities")
	}

	if frameworkActivities != nil {
		w.RegisterActivity(frameworkActivities.LearnFramework)
		w.RegisterActivity(frameworkActivities.GetFrameworkKnowledge)
		w.RegisterActivity(frameworkActivities.GenerateFrameworkPrompt)
		w.RegisterActivity(frameworkActivities.ListFrameworks)
		log.Info("Registered framework learning activities")
	}

	// Register storage activities
	if storageActivities != nil {
		w.RegisterActivity(storageActivities.Upload)
		w.RegisterActivity(storageActivities.Download)
		w.RegisterActivity(storageActivities.DeleteStorageFile)
		w.RegisterActivity(storageActivities.GetMetadata)
		w.RegisterActivity(storageActivities.GenerateUploadURL)
		w.RegisterActivity(storageActivities.GenerateDownloadURL)
		w.RegisterActivity(storageActivities.ListStorageFiles)
		w.RegisterActivity(storageActivities.CopyStorageFile)
		w.RegisterActivity(storageActivities.CreateBucket)
		w.RegisterActivity(storageActivities.ListBuckets)
		w.RegisterActivity(storageActivities.FileExists)
		log.Info("Registered cloud storage activities", logging.String("type", cfg.Storage.Type))
	}

	// Register RAG activities
	if ragActivities != nil {
		w.RegisterActivity(ragActivities.IndexDocument)
		w.RegisterActivity(ragActivities.Search)
		w.RegisterActivity(ragActivities.DeleteDocument)
		w.RegisterActivity(ragActivities.GenerateRAGPrompt)
		w.RegisterActivity(ragActivities.ListDocuments)
		log.Info("Registered RAG activities", logging.String("provider", cfg.RAG.Embedding.Provider))
	}

	// Start worker in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTime := time.Now()

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatal("Worker failed", logging.Error(err))
		}
	}()

	log.Info("Worker started",
		logging.String("task_queue", cfg.Temporal.TaskQueue),
		logging.String("temporal_address", cfg.Temporal.Address),
		logging.String("claude_provider", provider),
		logging.String("working_dir", workingDir),
		logging.String("version", version),
	)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info("Received shutdown signal",
			logging.String("signal", sig.String()),
			logging.Duration("uptime", time.Since(startTime)),
		)
		cancel()
	case <-ctx.Done():
	}

	log.Info("Worker stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
