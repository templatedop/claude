package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// HTTPMiddleware wraps an http.Handler and records metrics
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status and size
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Get request size
		requestSize := r.ContentLength
		if requestSize < 0 {
			requestSize = 0
		}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(wrapped.statusCode)

		m.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
		m.HTTPRequestSize.WithLabelValues(r.Method, path).Observe(float64(requestSize))
		m.HTTPResponseSize.WithLabelValues(r.Method, path).Observe(float64(wrapped.size))
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += int64(size)
	return size, err
}

// Unwrap returns the original ResponseWriter for compatibility
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// normalizePath normalizes URL paths for metric labels
// This prevents high cardinality from dynamic path segments
func normalizePath(path string) string {
	// Common API path patterns
	patterns := map[string]string{
		"/api/v1/workflows/":  "/api/v1/workflows/{id}",
		"/api/v1/tasks/":      "/api/v1/tasks/{id}",
		"/api/v1/agents/":     "/api/v1/agents/{id}",
		"/api/v1/memory/":     "/api/v1/memory/{key}",
		"/api/v1/storage/":    "/api/v1/storage/{path}",
		"/api/v1/rag/":        "/api/v1/rag/{id}",
	}

	for prefix, normalized := range patterns {
		if len(path) > len(prefix) && path[:len(prefix)] == prefix {
			return normalized
		}
	}

	return path
}

// RecordWorkflowStart records a workflow start
func (m *Metrics) RecordWorkflowStart(workflowType string) {
	m.WorkflowsStarted.WithLabelValues(workflowType).Inc()
	m.WorkflowsActive.WithLabelValues(workflowType).Inc()
}

// RecordWorkflowComplete records a workflow completion
func (m *Metrics) RecordWorkflowComplete(workflowType string, duration time.Duration) {
	m.WorkflowsCompleted.WithLabelValues(workflowType).Inc()
	m.WorkflowsActive.WithLabelValues(workflowType).Dec()
	m.WorkflowDuration.WithLabelValues(workflowType, "completed").Observe(duration.Seconds())
}

// RecordWorkflowFailed records a workflow failure
func (m *Metrics) RecordWorkflowFailed(workflowType, errorType string, duration time.Duration) {
	m.WorkflowsFailed.WithLabelValues(workflowType, errorType).Inc()
	m.WorkflowsActive.WithLabelValues(workflowType).Dec()
	m.WorkflowDuration.WithLabelValues(workflowType, "failed").Observe(duration.Seconds())
}

// RecordActivityStart records an activity start
func (m *Metrics) RecordActivityStart(activityName string) {
	m.ActivitiesStarted.WithLabelValues(activityName).Inc()
	m.ActivitiesActive.WithLabelValues(activityName).Inc()
}

// RecordActivityComplete records an activity completion
func (m *Metrics) RecordActivityComplete(activityName string, duration time.Duration) {
	m.ActivitiesCompleted.WithLabelValues(activityName).Inc()
	m.ActivitiesActive.WithLabelValues(activityName).Dec()
	m.ActivityDuration.WithLabelValues(activityName, "completed").Observe(duration.Seconds())
}

// RecordActivityFailed records an activity failure
func (m *Metrics) RecordActivityFailed(activityName, errorType string, duration time.Duration) {
	m.ActivitiesFailed.WithLabelValues(activityName, errorType).Inc()
	m.ActivitiesActive.WithLabelValues(activityName).Dec()
	m.ActivityDuration.WithLabelValues(activityName, "failed").Observe(duration.Seconds())
}

// RecordAgentTask records an agent task assignment
func (m *Metrics) RecordAgentTask(agentType string) {
	m.AgentTasksAssigned.WithLabelValues(agentType).Inc()
}

// RecordAgentTaskComplete records an agent task completion
func (m *Metrics) RecordAgentTaskComplete(agentType string, duration time.Duration) {
	m.AgentTasksCompleted.WithLabelValues(agentType).Inc()
	m.AgentTaskDuration.WithLabelValues(agentType, "completed").Observe(duration.Seconds())
}

// RecordAgentTaskFailed records an agent task failure
func (m *Metrics) RecordAgentTaskFailed(agentType, errorType string, duration time.Duration) {
	m.AgentTasksFailed.WithLabelValues(agentType, errorType).Inc()
	m.AgentTaskDuration.WithLabelValues(agentType, "failed").Observe(duration.Seconds())
}

// RecordClaudeAPIRequest records a Claude API request
func (m *Metrics) RecordClaudeAPIRequest(model, endpoint string, latency time.Duration, inputTokens, outputTokens int) {
	m.ClaudeAPIRequests.WithLabelValues(model, endpoint).Inc()
	m.ClaudeAPILatency.WithLabelValues(model).Observe(latency.Seconds())
	m.ClaudeAPITokensInput.WithLabelValues(model).Add(float64(inputTokens))
	m.ClaudeAPITokensOutput.WithLabelValues(model).Add(float64(outputTokens))
}

// RecordClaudeAPIError records a Claude API error
func (m *Metrics) RecordClaudeAPIError(model, errorType string) {
	m.ClaudeAPIErrors.WithLabelValues(model, errorType).Inc()
}

// RecordMemoryOperation records a memory operation
func (m *Metrics) RecordMemoryOperation(operation, namespace string, duration time.Duration) {
	m.MemoryOperations.WithLabelValues(operation, namespace).Inc()
	m.MemoryLatency.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordMemoryHit records a memory cache hit
func (m *Metrics) RecordMemoryHit(namespace string) {
	m.MemoryHits.WithLabelValues(namespace).Inc()
}

// RecordMemoryMiss records a memory cache miss
func (m *Metrics) RecordMemoryMiss(namespace string) {
	m.MemoryMisses.WithLabelValues(namespace).Inc()
}

// SetMemorySize sets the memory size gauge
func (m *Metrics) SetMemorySize(namespace, memType string, size int) {
	m.MemorySize.WithLabelValues(namespace, memType).Set(float64(size))
}

// RecordStorageOperation records a storage operation
func (m *Metrics) RecordStorageOperation(operation, backend string, duration time.Duration, bytesTransferred int64) {
	m.StorageOperations.WithLabelValues(operation, backend).Inc()
	m.StorageLatency.WithLabelValues(operation, backend).Observe(duration.Seconds())
	if operation == "upload" || operation == "write" {
		m.StorageBytesWritten.WithLabelValues(backend).Add(float64(bytesTransferred))
	} else if operation == "download" || operation == "read" {
		m.StorageBytesRead.WithLabelValues(backend).Add(float64(bytesTransferred))
	}
}

// RecordStorageError records a storage error
func (m *Metrics) RecordStorageError(operation, backend, errorType string) {
	m.StorageErrors.WithLabelValues(operation, backend, errorType).Inc()
}

// RecordRAGIndex records a RAG index operation
func (m *Metrics) RecordRAGIndex(operation, namespace string, chunksProcessed int) {
	m.RAGIndexOperations.WithLabelValues(operation, namespace).Inc()
}

// RecordRAGSearch records a RAG search operation
func (m *Metrics) RecordRAGSearch(namespace string, duration time.Duration) {
	m.RAGSearchOperations.WithLabelValues(namespace).Inc()
	m.RAGSearchLatency.WithLabelValues(namespace).Observe(duration.Seconds())
}

// SetRAGDocumentsIndexed sets the number of indexed documents
func (m *Metrics) SetRAGDocumentsIndexed(namespace string, count int) {
	m.RAGDocumentsIndexed.WithLabelValues(namespace).Set(float64(count))
}

// RecordRAGChunks records chunks processed
func (m *Metrics) RecordRAGChunks(strategy string, count int) {
	m.RAGChunksProcessed.WithLabelValues(strategy).Add(float64(count))
}

// SetInfo sets the application info gauge
func (m *Metrics) SetInfo(version, goVersion, buildTime string) {
	m.InfoGauge.WithLabelValues(version, goVersion, buildTime).Set(1)
}
