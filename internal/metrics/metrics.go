package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds metrics configuration
type Config struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	Address     string `json:"address" yaml:"address"` // Metrics server address (e.g., ":9090")
	Path        string `json:"path" yaml:"path"`       // Metrics endpoint path (default: /metrics)
	Namespace   string `json:"namespace" yaml:"namespace"` // Prometheus namespace
	Subsystem   string `json:"subsystem" yaml:"subsystem"` // Prometheus subsystem
	EnableGoMetrics bool `json:"enable_go_metrics" yaml:"enable_go_metrics"`
	EnableProcessMetrics bool `json:"enable_process_metrics" yaml:"enable_process_metrics"`
	Buckets     []float64 `json:"buckets" yaml:"buckets"` // Custom histogram buckets
}

// DefaultConfig returns default metrics configuration
func DefaultConfig() Config {
	return Config{
		Enabled:              true,
		Address:              ":9090",
		Path:                 "/metrics",
		Namespace:            "claude_orchestrator",
		Subsystem:            "",
		EnableGoMetrics:      true,
		EnableProcessMetrics: true,
		Buckets:              prometheus.DefBuckets,
	}
}

// Metrics holds all application metrics
type Metrics struct {
	config   Config
	registry *prometheus.Registry

	// Workflow metrics
	WorkflowsStarted   *prometheus.CounterVec
	WorkflowsCompleted *prometheus.CounterVec
	WorkflowsFailed    *prometheus.CounterVec
	WorkflowDuration   *prometheus.HistogramVec
	WorkflowsActive    *prometheus.GaugeVec

	// Activity metrics
	ActivitiesStarted   *prometheus.CounterVec
	ActivitiesCompleted *prometheus.CounterVec
	ActivitiesFailed    *prometheus.CounterVec
	ActivityDuration    *prometheus.HistogramVec
	ActivitiesActive    *prometheus.GaugeVec

	// Agent metrics
	AgentTasksAssigned  *prometheus.CounterVec
	AgentTasksCompleted *prometheus.CounterVec
	AgentTasksFailed    *prometheus.CounterVec
	AgentTaskDuration   *prometheus.HistogramVec

	// Claude API metrics
	ClaudeAPIRequests      *prometheus.CounterVec
	ClaudeAPIErrors        *prometheus.CounterVec
	ClaudeAPILatency       *prometheus.HistogramVec
	ClaudeAPITokensInput   *prometheus.CounterVec
	ClaudeAPITokensOutput  *prometheus.CounterVec

	// Memory metrics
	MemoryOperations *prometheus.CounterVec
	MemoryLatency    *prometheus.HistogramVec
	MemorySize       *prometheus.GaugeVec
	MemoryHits       *prometheus.CounterVec
	MemoryMisses     *prometheus.CounterVec

	// Storage metrics
	StorageOperations   *prometheus.CounterVec
	StorageErrors       *prometheus.CounterVec
	StorageLatency      *prometheus.HistogramVec
	StorageBytesRead    *prometheus.CounterVec
	StorageBytesWritten *prometheus.CounterVec

	// RAG metrics
	RAGIndexOperations  *prometheus.CounterVec
	RAGSearchOperations *prometheus.CounterVec
	RAGSearchLatency    *prometheus.HistogramVec
	RAGDocumentsIndexed *prometheus.GaugeVec
	RAGChunksProcessed  *prometheus.CounterVec

	// HTTP API metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// System metrics
	InfoGauge *prometheus.GaugeVec
}

var (
	globalMetrics *Metrics
	globalMu      sync.RWMutex
)

// New creates a new Metrics instance
func New(cfg Config) *Metrics {
	registry := prometheus.NewRegistry()

	// Register default collectors if enabled
	if cfg.EnableGoMetrics {
		registry.MustRegister(prometheus.NewGoCollector())
	}
	if cfg.EnableProcessMetrics {
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = "claude_orchestrator"
	}

	buckets := cfg.Buckets
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}

	factory := promauto.With(registry)

	m := &Metrics{
		config:   cfg,
		registry: registry,

		// Workflow metrics
		WorkflowsStarted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "workflows_started_total",
				Help:      "Total number of workflows started",
			},
			[]string{"workflow_type"},
		),
		WorkflowsCompleted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "workflows_completed_total",
				Help:      "Total number of workflows completed successfully",
			},
			[]string{"workflow_type"},
		),
		WorkflowsFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "workflows_failed_total",
				Help:      "Total number of workflows that failed",
			},
			[]string{"workflow_type", "error_type"},
		),
		WorkflowDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "workflow_duration_seconds",
				Help:      "Duration of workflow execution in seconds",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"workflow_type", "status"},
		),
		WorkflowsActive: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: ns,
				Name:      "workflows_active",
				Help:      "Number of currently active workflows",
			},
			[]string{"workflow_type"},
		),

		// Activity metrics
		ActivitiesStarted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "activities_started_total",
				Help:      "Total number of activities started",
			},
			[]string{"activity_name"},
		),
		ActivitiesCompleted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "activities_completed_total",
				Help:      "Total number of activities completed successfully",
			},
			[]string{"activity_name"},
		),
		ActivitiesFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "activities_failed_total",
				Help:      "Total number of activities that failed",
			},
			[]string{"activity_name", "error_type"},
		),
		ActivityDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "activity_duration_seconds",
				Help:      "Duration of activity execution in seconds",
				Buckets:   buckets,
			},
			[]string{"activity_name", "status"},
		),
		ActivitiesActive: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: ns,
				Name:      "activities_active",
				Help:      "Number of currently active activities",
			},
			[]string{"activity_name"},
		),

		// Agent metrics
		AgentTasksAssigned: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "agent_tasks_assigned_total",
				Help:      "Total number of tasks assigned to agents",
			},
			[]string{"agent_type"},
		),
		AgentTasksCompleted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "agent_tasks_completed_total",
				Help:      "Total number of tasks completed by agents",
			},
			[]string{"agent_type"},
		),
		AgentTasksFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "agent_tasks_failed_total",
				Help:      "Total number of tasks failed by agents",
			},
			[]string{"agent_type", "error_type"},
		),
		AgentTaskDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "agent_task_duration_seconds",
				Help:      "Duration of agent task execution in seconds",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
			},
			[]string{"agent_type", "status"},
		),

		// Claude API metrics
		ClaudeAPIRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "claude_api_requests_total",
				Help:      "Total number of Claude API requests",
			},
			[]string{"model", "endpoint"},
		),
		ClaudeAPIErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "claude_api_errors_total",
				Help:      "Total number of Claude API errors",
			},
			[]string{"model", "error_type"},
		),
		ClaudeAPILatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "claude_api_latency_seconds",
				Help:      "Latency of Claude API requests in seconds",
				Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"model"},
		),
		ClaudeAPITokensInput: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "claude_api_tokens_input_total",
				Help:      "Total number of input tokens sent to Claude API",
			},
			[]string{"model"},
		),
		ClaudeAPITokensOutput: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "claude_api_tokens_output_total",
				Help:      "Total number of output tokens received from Claude API",
			},
			[]string{"model"},
		),

		// Memory metrics
		MemoryOperations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "memory_operations_total",
				Help:      "Total number of memory operations",
			},
			[]string{"operation", "namespace"},
		),
		MemoryLatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "memory_operation_duration_seconds",
				Help:      "Duration of memory operations in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"operation"},
		),
		MemorySize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: ns,
				Name:      "memory_entries",
				Help:      "Number of entries in memory store",
			},
			[]string{"namespace", "type"},
		),
		MemoryHits: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "memory_hits_total",
				Help:      "Total number of memory cache hits",
			},
			[]string{"namespace"},
		),
		MemoryMisses: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "memory_misses_total",
				Help:      "Total number of memory cache misses",
			},
			[]string{"namespace"},
		),

		// Storage metrics
		StorageOperations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "storage_operations_total",
				Help:      "Total number of storage operations",
			},
			[]string{"operation", "backend"},
		),
		StorageErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "storage_errors_total",
				Help:      "Total number of storage errors",
			},
			[]string{"operation", "backend", "error_type"},
		),
		StorageLatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "storage_operation_duration_seconds",
				Help:      "Duration of storage operations in seconds",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"operation", "backend"},
		),
		StorageBytesRead: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "storage_bytes_read_total",
				Help:      "Total bytes read from storage",
			},
			[]string{"backend"},
		),
		StorageBytesWritten: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "storage_bytes_written_total",
				Help:      "Total bytes written to storage",
			},
			[]string{"backend"},
		),

		// RAG metrics
		RAGIndexOperations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "rag_index_operations_total",
				Help:      "Total number of RAG index operations",
			},
			[]string{"operation", "namespace"},
		),
		RAGSearchOperations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "rag_search_operations_total",
				Help:      "Total number of RAG search operations",
			},
			[]string{"namespace"},
		),
		RAGSearchLatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "rag_search_duration_seconds",
				Help:      "Duration of RAG search operations in seconds",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"namespace"},
		),
		RAGDocumentsIndexed: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: ns,
				Name:      "rag_documents_indexed",
				Help:      "Number of documents indexed in RAG",
			},
			[]string{"namespace"},
		),
		RAGChunksProcessed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "rag_chunks_processed_total",
				Help:      "Total number of document chunks processed",
			},
			[]string{"strategy"},
		),

		// HTTP API metrics
		HTTPRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "http_request_duration_seconds",
				Help:      "Duration of HTTP requests in seconds",
				Buckets:   buckets,
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "http_request_size_bytes",
				Help:      "Size of HTTP requests in bytes",
				Buckets:   []float64{100, 1000, 10000, 100000, 1000000},
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "http_response_size_bytes",
				Help:      "Size of HTTP responses in bytes",
				Buckets:   []float64{100, 1000, 10000, 100000, 1000000, 10000000},
			},
			[]string{"method", "path"},
		),

		// System metrics
		InfoGauge: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: ns,
				Name:      "info",
				Help:      "Application information",
			},
			[]string{"version", "go_version", "build_time"},
		),
	}

	return m
}

// SetGlobal sets the global metrics instance
func SetGlobal(m *Metrics) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalMetrics = m
}

// Global returns the global metrics instance
func Global() *Metrics {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalMetrics == nil {
		m := New(DefaultConfig())
		return m
	}
	return globalMetrics
}

// Handler returns the Prometheus HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Registry returns the Prometheus registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// StartServer starts the metrics HTTP server
func (m *Metrics) StartServer() error {
	if !m.config.Enabled {
		return nil
	}

	path := m.config.Path
	if path == "" {
		path = "/metrics"
	}

	mux := http.NewServeMux()
	mux.Handle(path, m.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    m.config.Address,
		Handler: mux,
	}

	return server.ListenAndServe()
}

// Timer is a helper for timing operations
type Timer struct {
	start    time.Time
	observer prometheus.Observer
}

// NewTimer creates a new timer
func NewTimer(observer prometheus.Observer) *Timer {
	return &Timer{
		start:    time.Now(),
		observer: observer,
	}
}

// ObserveDuration records the duration since the timer was created
func (t *Timer) ObserveDuration() time.Duration {
	d := time.Since(t.start)
	t.observer.Observe(d.Seconds())
	return d
}

// ObserveDurationWithLabels records duration for a histogram with labels
func ObserveDurationWithLabels(hist *prometheus.HistogramVec, labels prometheus.Labels) func() {
	start := time.Now()
	return func() {
		hist.With(labels).Observe(time.Since(start).Seconds())
	}
}
