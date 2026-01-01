// Package config provides configuration management for the orchestrator.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config represents the complete orchestrator configuration.
type Config struct {
	// Core settings
	Temporal      TemporalConfig      `json:"temporal"`
	Claude        ClaudeConfig        `json:"claude"`
	Memory        MemoryConfig        `json:"memory"`

	// Feature toggles
	Features      FeatureConfig       `json:"features"`

	// Agent configurations
	Agents        AgentConfigs        `json:"agents"`

	// Workflow settings
	Workflow      WorkflowConfig      `json:"workflow"`
}

// TemporalConfig contains Temporal server configuration.
type TemporalConfig struct {
	Address   string `json:"address" env:"TEMPORAL_ADDRESS"`
	Namespace string `json:"namespace" env:"TEMPORAL_NAMESPACE"`
	TaskQueue string `json:"task_queue" env:"TASK_QUEUE"`
}

// ClaudeConfig contains Claude API configuration.
type ClaudeConfig struct {
	APIKey      string  `json:"api_key" env:"ANTHROPIC_API_KEY"`
	Model       string  `json:"model" env:"CLAUDE_MODEL"`
	MaxTokens   int     `json:"max_tokens" env:"CLAUDE_MAX_TOKENS"`
	Temperature float64 `json:"temperature" env:"CLAUDE_TEMPERATURE"`
}

// MemoryConfig contains memory store configuration.
type MemoryConfig struct {
	Type     string         `json:"type" env:"MEMORY_TYPE"` // inmemory, postgres
	Postgres PostgresConfig `json:"postgres,omitempty"`
}

// PostgresConfig contains PostgreSQL configuration.
type PostgresConfig struct {
	Host     string `json:"host" env:"POSTGRES_HOST"`
	Port     int    `json:"port" env:"POSTGRES_PORT"`
	Database string `json:"database" env:"POSTGRES_DB"`
	User     string `json:"user" env:"POSTGRES_USER"`
	Password string `json:"password" env:"POSTGRES_PASSWORD"`
	SSLMode  string `json:"ssl_mode" env:"POSTGRES_SSLMODE"`
}

// FeatureConfig contains feature toggle configuration.
type FeatureConfig struct {
	// Document Analysis
	DocumentAnalysis DocumentAnalysisFeature `json:"document_analysis"`

	// Requirements Tracking
	RequirementsTracking RequirementsTrackingFeature `json:"requirements_tracking"`

	// Framework Learning
	FrameworkLearning FrameworkLearningFeature `json:"framework_learning"`

	// Code Review
	CodeReview bool `json:"code_review" env:"FEATURE_CODE_REVIEW"`

	// Parallel Execution
	ParallelExecution bool `json:"parallel_execution" env:"FEATURE_PARALLEL_EXECUTION"`
}

// DocumentAnalysisFeature contains document analysis feature configuration.
type DocumentAnalysisFeature struct {
	Enabled          bool     `json:"enabled" env:"FEATURE_DOC_ANALYSIS"`
	SupportedFormats []string `json:"supported_formats"`
	MaxFileSizeMB    int      `json:"max_file_size_mb"`
	ExtractItems     []string `json:"extract_items"` // requirements, features, constraints, etc.
}

// RequirementsTrackingFeature contains requirements tracking feature configuration.
type RequirementsTrackingFeature struct {
	Enabled              bool `json:"enabled" env:"FEATURE_REQ_TRACKING"`
	AutoDetect           bool `json:"auto_detect"`           // Auto-detect requirements from documents
	TrackImplementation  bool `json:"track_implementation"`  // Track implementation status
	TrackTests           bool `json:"track_tests"`           // Track test coverage
	HistoryRetentionDays int  `json:"history_retention_days"`
}

// FrameworkLearningFeature contains framework learning feature configuration.
type FrameworkLearningFeature struct {
	Enabled           bool     `json:"enabled" env:"FEATURE_FRAMEWORK_LEARNING"`
	AutoLearn         bool     `json:"auto_learn"`           // Auto-learn from project dependencies
	MaxSourceFiles    int      `json:"max_source_files"`     // Max files to analyze from codebase
	SupportedLanguages []string `json:"supported_languages"`
	CacheExpiryDays   int      `json:"cache_expiry_days"`
}

// AgentConfigs contains configuration for all agent types.
type AgentConfigs struct {
	Orchestrator AgentConfig `json:"orchestrator"`
	Planner      AgentConfig `json:"planner"`
	Researcher   AgentConfig `json:"researcher"`
	Coder        AgentConfig `json:"coder"`
	Reviewer     AgentConfig `json:"reviewer"`
	Executor     AgentConfig `json:"executor"`
}

// AgentConfig contains configuration for a single agent.
type AgentConfig struct {
	Enabled      bool    `json:"enabled"`
	Model        string  `json:"model"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
	SystemPrompt string  `json:"system_prompt,omitempty"`
}

// WorkflowConfig contains workflow execution configuration.
type WorkflowConfig struct {
	DefaultTimeout    time.Duration `json:"default_timeout"`
	MaxAgents         int           `json:"max_agents"`
	MaxRetries        int           `json:"max_retries"`
	RetryInterval     time.Duration `json:"retry_interval"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Temporal: TemporalConfig{
			Address:   "localhost:7233",
			Namespace: "default",
			TaskQueue: "claude-orchestrator",
		},
		Claude: ClaudeConfig{
			Model:       "claude-sonnet-4-20250514",
			MaxTokens:   4096,
			Temperature: 0.7,
		},
		Memory: MemoryConfig{
			Type: "inmemory",
			Postgres: PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "claude_orchestrator",
				User:     "postgres",
				SSLMode:  "disable",
			},
		},
		Features: FeatureConfig{
			DocumentAnalysis: DocumentAnalysisFeature{
				Enabled:          true,
				SupportedFormats: []string{".md", ".txt", ".rst", ".adoc", ".pdf", ".docx"},
				MaxFileSizeMB:    10,
				ExtractItems:     []string{"requirements", "features", "constraints", "dependencies"},
			},
			RequirementsTracking: RequirementsTrackingFeature{
				Enabled:              true,
				AutoDetect:           true,
				TrackImplementation:  true,
				TrackTests:           true,
				HistoryRetentionDays: 90,
			},
			FrameworkLearning: FrameworkLearningFeature{
				Enabled:            true,
				AutoLearn:          true,
				MaxSourceFiles:     50,
				SupportedLanguages: []string{"go", "javascript", "typescript", "python", "java", "rust"},
				CacheExpiryDays:    30,
			},
			CodeReview:        true,
			ParallelExecution: true,
		},
		Agents: AgentConfigs{
			Orchestrator: AgentConfig{
				Enabled:     true,
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   4096,
				Temperature: 0.3,
			},
			Planner: AgentConfig{
				Enabled:     true,
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   4096,
				Temperature: 0.4,
			},
			Researcher: AgentConfig{
				Enabled:     true,
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   4096,
				Temperature: 0.5,
			},
			Coder: AgentConfig{
				Enabled:     true,
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   8192,
				Temperature: 0.2,
			},
			Reviewer: AgentConfig{
				Enabled:     true,
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   4096,
				Temperature: 0.3,
			},
			Executor: AgentConfig{
				Enabled:     true,
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   2048,
				Temperature: 0.1,
			},
		},
		Workflow: WorkflowConfig{
			DefaultTimeout:    60 * time.Minute,
			MaxAgents:         10,
			MaxRetries:        3,
			RetryInterval:     time.Second,
			HeartbeatInterval: 2 * time.Minute,
		},
	}
}

// LoadConfig loads configuration from a file and environment variables.
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	// Load from file if provided
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables
	applyEnvironmentOverrides(config)

	return config, nil
}

// SaveConfig saves configuration to a file.
func SaveConfig(config *Config, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// applyEnvironmentOverrides applies environment variable overrides to config.
func applyEnvironmentOverrides(config *Config) {
	// Temporal
	if v := os.Getenv("TEMPORAL_ADDRESS"); v != "" {
		config.Temporal.Address = v
	}
	if v := os.Getenv("TEMPORAL_NAMESPACE"); v != "" {
		config.Temporal.Namespace = v
	}
	if v := os.Getenv("TASK_QUEUE"); v != "" {
		config.Temporal.TaskQueue = v
	}

	// Claude
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		config.Claude.APIKey = v
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		config.Claude.Model = v
	}

	// Memory
	if v := os.Getenv("MEMORY_TYPE"); v != "" {
		config.Memory.Type = v
	}

	// PostgreSQL
	if v := os.Getenv("POSTGRES_HOST"); v != "" {
		config.Memory.Postgres.Host = v
	}
	if v := os.Getenv("POSTGRES_USER"); v != "" {
		config.Memory.Postgres.User = v
	}
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		config.Memory.Postgres.Password = v
	}
	if v := os.Getenv("POSTGRES_DB"); v != "" {
		config.Memory.Postgres.Database = v
	}

	// Feature toggles
	if v := os.Getenv("FEATURE_DOC_ANALYSIS"); v != "" {
		config.Features.DocumentAnalysis.Enabled = parseBool(v)
	}
	if v := os.Getenv("FEATURE_REQ_TRACKING"); v != "" {
		config.Features.RequirementsTracking.Enabled = parseBool(v)
	}
	if v := os.Getenv("FEATURE_FRAMEWORK_LEARNING"); v != "" {
		config.Features.FrameworkLearning.Enabled = parseBool(v)
	}
	if v := os.Getenv("FEATURE_CODE_REVIEW"); v != "" {
		config.Features.CodeReview = parseBool(v)
	}
	if v := os.Getenv("FEATURE_PARALLEL_EXECUTION"); v != "" {
		config.Features.ParallelExecution = parseBool(v)
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Claude.APIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required")
	}

	if c.Temporal.Address == "" {
		return fmt.Errorf("Temporal address is required")
	}

	if c.Workflow.MaxAgents <= 0 {
		return fmt.Errorf("max_agents must be positive")
	}

	return nil
}

// IsFeatureEnabled checks if a feature is enabled.
func (c *Config) IsFeatureEnabled(feature string) bool {
	switch strings.ToLower(feature) {
	case "document_analysis", "doc_analysis":
		return c.Features.DocumentAnalysis.Enabled
	case "requirements_tracking", "req_tracking":
		return c.Features.RequirementsTracking.Enabled
	case "framework_learning":
		return c.Features.FrameworkLearning.Enabled
	case "code_review":
		return c.Features.CodeReview
	case "parallel_execution":
		return c.Features.ParallelExecution
	default:
		return false
	}
}

// GetAgentConfig returns the configuration for a specific agent type.
func (c *Config) GetAgentConfig(agentType string) AgentConfig {
	switch strings.ToLower(agentType) {
	case "orchestrator":
		return c.Agents.Orchestrator
	case "planner":
		return c.Agents.Planner
	case "researcher":
		return c.Agents.Researcher
	case "coder":
		return c.Agents.Coder
	case "reviewer":
		return c.Agents.Reviewer
	case "executor":
		return c.Agents.Executor
	default:
		return AgentConfig{Enabled: false}
	}
}
