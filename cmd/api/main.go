// Package main provides a REST API server for the Claude orchestrator.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/config"
	"github.com/anthropics/claude-orchestrator/internal/domain"
	"github.com/anthropics/claude-orchestrator/internal/logging"
	"github.com/anthropics/claude-orchestrator/internal/metrics"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	"github.com/google/uuid"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var (
	version        = "dev"
	buildTime      = "unknown"
	temporalClient client.Client
	appMetrics     *metrics.Metrics
	log            *logging.Logger
)

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.LoadConfig(configPath)
	if err != nil && configPath != "" {
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

	log = logger.Named("api")

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
	appMetrics = metrics.New(metricsCfg)
	metrics.SetGlobal(appMetrics)

	// Set application info
	appMetrics.SetInfo(version, runtime.Version(), buildTime)

	// Get configuration from environment (for backwards compatibility)
	port := getEnv("PORT", "8080")
	temporalAddr := getEnv("TEMPORAL_ADDRESS", cfg.Temporal.Address)
	temporalNS := getEnv("TEMPORAL_NAMESPACE", cfg.Temporal.Namespace)

	// Create Temporal client
	temporalClient, err = client.Dial(client.Options{
		HostPort:  temporalAddr,
		Namespace: temporalNS,
	})
	if err != nil {
		log.Fatal("Failed to create Temporal client", logging.Error(err))
	}
	defer temporalClient.Close()

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)
	mux.HandleFunc("/api/v1/workflows", workflowsHandler)
	mux.HandleFunc("/api/v1/workflows/", workflowHandler)

	// Add metrics endpoint if not using separate metrics server
	if !cfg.Metrics.Enabled {
		mux.Handle("/metrics", appMetrics.Handler())
	}

	// Apply middleware
	var handler http.Handler = mux
	handler = appMetrics.HTTPMiddleware(handler) // Metrics middleware
	handler = loggingMiddleware(handler)         // Logging middleware
	handler = corsMiddleware(handler)            // CORS middleware

	// Create server
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start metrics server in background if enabled (separate port)
	if cfg.Metrics.Enabled {
		go func() {
			log.Info("Starting metrics server",
				logging.String("address", cfg.Metrics.Address),
				logging.String("path", cfg.Metrics.Path),
			)
			if err := appMetrics.StartServer(); err != nil {
				log.Error("Metrics server failed", logging.Error(err))
			}
		}()
	}

	// Start API server
	startTime := time.Now()
	go func() {
		log.Info("API server starting",
			logging.String("port", port),
			logging.String("temporal_address", temporalAddr),
			logging.String("version", version),
		)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("Server failed", logging.Error(err))
		}
	}()

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	log.Info("Received shutdown signal",
		logging.String("signal", sig.String()),
		logging.Duration("uptime", time.Since(startTime)),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("Server shutdown error", logging.Error(err))
	}

	log.Info("Server stopped")
}

// CreateWorkflowRequest represents a request to create a workflow.
type CreateWorkflowRequest struct {
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	MaxAgents   int                    `json:"max_agents,omitempty"`
	Timeout     int                    `json:"timeout_minutes,omitempty"`
	Parallel    bool                   `json:"parallel"`
	Review      bool                   `json:"review"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// WorkflowResponse represents a workflow in the API response.
type WorkflowResponse struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	TaskTitle  string `json:"task_title,omitempty"`
	StartTime  string `json:"start_time,omitempty"`
	EndTime    string `json:"end_time,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	// Check Temporal connection
	_, err := temporalClient.CheckHealth(r.Context(), nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
			"error":  "temporal connection failed",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func workflowsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createWorkflow(w, r)
	case http.MethodGet:
		listWorkflows(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func workflowHandler(w http.ResponseWriter, r *http.Request) {
	// Extract workflow ID from path
	workflowID := r.URL.Path[len("/api/v1/workflows/"):]
	if workflowID == "" {
		writeError(w, http.StatusBadRequest, "workflow_id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		getWorkflow(w, r, workflowID)
	case http.MethodDelete:
		cancelWorkflow(w, r, workflowID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createWorkflow(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	if req.Description == "" {
		req.Description = req.Title
	}

	if req.MaxAgents == 0 {
		req.MaxAgents = 10
	}

	if req.Timeout == 0 {
		req.Timeout = 60
	}

	// Create task
	taskID := uuid.New().String()
	task := domain.Task{
		ID:          taskID,
		Title:       req.Title,
		Description: req.Description,
		Status:      domain.TaskStatusPending,
		Priority:    domain.TaskPriorityNormal,
		CreatedAt:   time.Now(),
		Context: domain.TaskContext{
			WorkingDir: ".",
			TimeoutSec: req.Timeout * 60,
		},
	}

	// Create input
	input := workflow.OrchestratorInput{
		Task: task,
		Config: workflow.OrchestratorConfig{
			MaxAgents:      req.MaxAgents,
			MaxRetries:     3,
			TimeoutMinutes: req.Timeout,
			EnableReview:   req.Review,
			ParallelTasks:  req.Parallel,
		},
		Context: req.Context,
	}

	// Start workflow
	workflowID := fmt.Sprintf("orchestrator-%s", taskID)
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: workflow.TaskQueueName,
	}

	we, err := temporalClient.ExecuteWorkflow(context.Background(), options, workflow.OrchestratorWorkflow, input)
	if err != nil {
		log.Error("Failed to start workflow",
			logging.Error(err),
			logging.WorkflowID(workflowID),
		)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to start workflow: %v", err))
		return
	}

	// Record workflow started metric
	appMetrics.RecordWorkflowStart("orchestrator")

	log.Info("Workflow started",
		logging.WorkflowID(we.GetID()),
		logging.RunID(we.GetRunID()),
		logging.String("title", req.Title),
	)

	response := WorkflowResponse{
		WorkflowID: we.GetID(),
		RunID:      we.GetRunID(),
		Status:     "Running",
		TaskTitle:  req.Title,
		StartTime:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func listWorkflows(w http.ResponseWriter, r *http.Request) {
	query := "WorkflowType = 'OrchestratorWorkflow'"

	if status := r.URL.Query().Get("status"); status != "" {
		query += fmt.Sprintf(" AND ExecutionStatus = '%s'", status)
	}

	temporalNS := getEnv("TEMPORAL_NAMESPACE", "default")
	resp, err := temporalClient.ListWorkflow(context.Background(), &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: temporalNS,
		Query:     query,
		PageSize:  50,
	})
	if err != nil {
		log.Error("Failed to list workflows", logging.Error(err))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list workflows: %v", err))
		return
	}

	workflows := make([]WorkflowResponse, 0)
	for _, exec := range resp.Executions {
		wf := WorkflowResponse{
			WorkflowID: exec.Execution.WorkflowId,
			RunID:      exec.Execution.RunId,
			Status:     exec.Status.String(),
			StartTime:  exec.StartTime.AsTime().Format(time.RFC3339),
		}
		if exec.CloseTime != nil {
			wf.EndTime = exec.CloseTime.AsTime().Format(time.RFC3339)
		}
		workflows = append(workflows, wf)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workflows)
}

func getWorkflow(w http.ResponseWriter, r *http.Request, workflowID string) {
	resp, err := temporalClient.DescribeWorkflowExecution(context.Background(), workflowID, "")
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Workflow not found: %v", err))
		return
	}

	info := resp.WorkflowExecutionInfo
	response := WorkflowResponse{
		WorkflowID: info.Execution.WorkflowId,
		RunID:      info.Execution.RunId,
		Status:     info.Status.String(),
		StartTime:  info.StartTime.AsTime().Format(time.RFC3339),
	}

	if info.CloseTime != nil {
		response.EndTime = info.CloseTime.AsTime().Format(time.RFC3339)
	}

	// If completed, try to get result
	if info.Status.String() == "Completed" {
		run := temporalClient.GetWorkflow(context.Background(), workflowID, "")
		var result workflow.OrchestratorOutput
		if err := run.Get(context.Background(), &result); err == nil {
			// Include result in response as additional field
			w.Header().Set("Content-Type", "application/json")
			fullResp := map[string]interface{}{
				"workflow": response,
				"result":   result,
			}
			json.NewEncoder(w).Encode(fullResp)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func cancelWorkflow(w http.ResponseWriter, r *http.Request, workflowID string) {
	err := temporalClient.CancelWorkflow(context.Background(), workflowID, "")
	if err != nil {
		log.Error("Failed to cancel workflow",
			logging.Error(err),
			logging.WorkflowID(workflowID),
		)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to cancel workflow: %v", err))
		return
	}

	log.Info("Workflow cancelled", logging.WorkflowID(workflowID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message":     "Workflow cancelled",
		"workflow_id": workflowID,
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Debug("HTTP request",
			logging.Method(r.Method),
			logging.URL(r.URL.Path),
			logging.Latency(time.Since(start)),
		)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
