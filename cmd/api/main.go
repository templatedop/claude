// Package main provides a REST API server for the Claude orchestrator.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/domain"
	"github.com/anthropics/claude-orchestrator/internal/workflow"
	"github.com/google/uuid"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

var temporalClient client.Client

func main() {
	// Get configuration from environment
	port := getEnv("PORT", "8080")
	temporalAddr := getEnv("TEMPORAL_ADDRESS", "localhost:7233")
	temporalNS := getEnv("TEMPORAL_NAMESPACE", "default")

	// Create Temporal client
	var err error
	temporalClient, err = client.Dial(client.Options{
		HostPort:  temporalAddr,
		Namespace: temporalNS,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer temporalClient.Close()

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/v1/workflows", workflowsHandler)
	mux.HandleFunc("/api/v1/workflows/", workflowHandler)

	// Create server
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      corsMiddleware(loggingMiddleware(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server
	go func() {
		log.Printf("API server starting on port %s", port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
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
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to start workflow: %v", err))
		return
	}

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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to cancel workflow: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Workflow cancelled",
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
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
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
