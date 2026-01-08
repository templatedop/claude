// Package config provides configuration management for the orchestrator.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete orchestrator configuration.
type Config struct {
	// Core settings
	Temporal TemporalConfig `json:"temporal" yaml:"temporal"`
	Claude   ClaudeConfig   `json:"claude" yaml:"claude"`
	Memory   MemoryConfig   `json:"memory" yaml:"memory"`
	Storage  StorageConfig  `json:"storage" yaml:"storage"`
	RAG      RAGConfig      `json:"rag" yaml:"rag"`

	// MCP (Model Context Protocol) servers
	MCP MCPConfig `json:"mcp" yaml:"mcp"`

	// LSP (Language Server Protocol) settings
	LSP LSPConfig `json:"lsp" yaml:"lsp"`

	// Observability
	Logging LoggingConfig `json:"logging" yaml:"logging"`
	Metrics MetricsConfig `json:"metrics" yaml:"metrics"`

	// Feature toggles
	Features FeatureConfig `json:"features" yaml:"features"`

	// Agent configurations
	Agents AgentConfigs `json:"agents" yaml:"agents"`

	// Skills configurations (reusable skill definitions)
	Skills map[string]SkillConfig `json:"skills" yaml:"skills"`

	// Workflow settings
	Workflow WorkflowConfig `json:"workflow" yaml:"workflow"`
}

// MCPConfig contains MCP server configurations.
type MCPConfig struct {
	// Enabled enables MCP server functionality
	Enabled bool `json:"enabled" yaml:"enabled"`

	// GitLab MCP server configuration
	GitLab GitLabMCPConfig `json:"gitlab" yaml:"gitlab"`

	// Database MCP server configuration
	Database DatabaseMCPConfig `json:"database" yaml:"database"`
}

// GitLabMCPConfig contains GitLab MCP server settings.
type GitLabMCPConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	BaseURL        string `json:"base_url" yaml:"base_url" env:"GITLAB_URL"`
	Token          string `json:"token" yaml:"token" env:"GITLAB_TOKEN"`
	DefaultProject string `json:"default_project" yaml:"default_project" env:"GITLAB_PROJECT"`
}

// DatabaseMCPConfig contains Database MCP server settings.
type DatabaseMCPConfig struct {
	Enabled       bool     `json:"enabled" yaml:"enabled"`
	Driver        string   `json:"driver" yaml:"driver"`
	Host          string   `json:"host" yaml:"host" env:"DB_HOST"`
	Port          int      `json:"port" yaml:"port" env:"DB_PORT"`
	User          string   `json:"user" yaml:"user" env:"DB_USER"`
	Password      string   `json:"password" yaml:"password" env:"DB_PASSWORD"`
	Database      string   `json:"database" yaml:"database" env:"DB_NAME"`
	SSLMode       string   `json:"ssl_mode" yaml:"ssl_mode"`
	MaxRows       int      `json:"max_rows" yaml:"max_rows"`
	ReadOnly      bool     `json:"read_only" yaml:"read_only"`
	AllowedTables []string `json:"allowed_tables" yaml:"allowed_tables"`
}

// LSPConfig contains LSP server configuration.
type LSPConfig struct {
	// Enabled enables the LSP server
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Address is the LSP server address (for TCP mode)
	Address string `json:"address" yaml:"address"`

	// Languages specifies which language handlers to enable
	Languages []string `json:"languages" yaml:"languages"`

	// Features specifies which LSP features to enable
	Features LSPFeatures `json:"features" yaml:"features"`
}

// LSPFeatures specifies enabled LSP features.
type LSPFeatures struct {
	Hover       bool `json:"hover" yaml:"hover"`
	Completion  bool `json:"completion" yaml:"completion"`
	Definition  bool `json:"definition" yaml:"definition"`
	References  bool `json:"references" yaml:"references"`
	Symbols     bool `json:"symbols" yaml:"symbols"`
	Diagnostics bool `json:"diagnostics" yaml:"diagnostics"`
}

// SkillConfig defines a reusable skill that can be assigned to agents.
type SkillConfig struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Tools       []string          `json:"tools" yaml:"tools"`
	Prompts     SkillPrompts      `json:"prompts" yaml:"prompts"`
	Commands    []string          `json:"commands,omitempty" yaml:"commands,omitempty"`
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	Options     map[string]string `json:"options,omitempty" yaml:"options,omitempty"`
}

// SkillPrompts contains before/after prompts for a skill.
type SkillPrompts struct {
	Before string `json:"before,omitempty" yaml:"before,omitempty"`
	After  string `json:"after,omitempty" yaml:"after,omitempty"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	Level         string `json:"level" env:"LOG_LEVEL"`           // debug, info, warn, error
	Format        string `json:"format" env:"LOG_FORMAT"`         // json, console, text
	Output        string `json:"output" env:"LOG_OUTPUT"`         // stdout, stderr, file path
	AddCaller     bool   `json:"add_caller"`
	AddStacktrace bool   `json:"add_stacktrace"`
	Development   bool   `json:"development" env:"LOG_DEVELOPMENT"`
	// Sampling configuration for high-volume logs
	Sampling      *LogSamplingConfig `json:"sampling,omitempty"`
}

// LogSamplingConfig configures log sampling.
type LogSamplingConfig struct {
	Enabled    bool `json:"enabled"`
	Initial    int  `json:"initial"`    // Log first N entries per second
	Thereafter int  `json:"thereafter"` // Then log every Mth entry
}

// MetricsConfig contains metrics configuration.
type MetricsConfig struct {
	Enabled              bool      `json:"enabled" env:"METRICS_ENABLED"`
	Address              string    `json:"address" env:"METRICS_ADDRESS"` // e.g., ":9090"
	Path                 string    `json:"path" env:"METRICS_PATH"`       // e.g., "/metrics"
	Namespace            string    `json:"namespace"`                     // Prometheus namespace
	EnableGoMetrics      bool      `json:"enable_go_metrics"`
	EnableProcessMetrics bool      `json:"enable_process_metrics"`
	Buckets              []float64 `json:"buckets,omitempty"` // Custom histogram buckets
}

// TemporalConfig contains Temporal server configuration.
type TemporalConfig struct {
	Address   string `json:"address" yaml:"address" env:"TEMPORAL_ADDRESS"`
	Namespace string `json:"namespace" yaml:"namespace" env:"TEMPORAL_NAMESPACE"`
	TaskQueue string `json:"task_queue" yaml:"task_queue" env:"TASK_QUEUE"`
}

// ClaudeConfig contains Claude API configuration.
type ClaudeConfig struct {
	// Provider specifies which Claude provider to use: "api" or "claude_code"
	// - "api": Uses Anthropic API directly (requires API credits)
	// - "claude_code": Uses Claude Code CLI (uses your Claude subscription)
	Provider    string  `json:"provider" yaml:"provider" env:"CLAUDE_PROVIDER"`

	// APIKey is required when using the "api" provider
	APIKey      string  `json:"api_key" yaml:"api_key" env:"ANTHROPIC_API_KEY"`

	// Model to use for completions
	Model       string  `json:"model" yaml:"model" env:"CLAUDE_MODEL"`
	MaxTokens   int     `json:"max_tokens" yaml:"max_tokens" env:"CLAUDE_MAX_TOKENS"`
	Temperature float64 `json:"temperature" yaml:"temperature" env:"CLAUDE_TEMPERATURE"`

	// WorkingDir is the working directory for Claude Code operations
	// Only used when Provider is "claude_code"
	WorkingDir  string  `json:"working_dir" yaml:"working_dir" env:"WORKING_DIR"`

	// AllowedTools specifies which tools Claude Code can use
	// Only used when Provider is "claude_code"
	// Default: ["Read", "Write", "Bash", "Glob", "Grep"]
	AllowedTools []string `json:"allowed_tools" yaml:"allowed_tools" env:"CLAUDE_ALLOWED_TOOLS"`
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

// StorageConfig contains file storage configuration.
type StorageConfig struct {
	Enabled  bool        `json:"enabled" env:"FEATURE_STORAGE"`
	Type     string      `json:"type" env:"STORAGE_TYPE"` // local, s3, gcs, azure, minio
	Local    LocalStorageConfig `json:"local,omitempty"`
	S3       S3StorageConfig    `json:"s3,omitempty"`
}

// LocalStorageConfig contains local file storage configuration.
type LocalStorageConfig struct {
	BasePath    string `json:"base_path" env:"STORAGE_LOCAL_PATH"`
	BaseURL     string `json:"base_url" env:"STORAGE_LOCAL_URL"`
	MaxFileSize int64  `json:"max_file_size"` // in bytes
}

// S3StorageConfig contains S3/MinIO storage configuration.
type S3StorageConfig struct {
	Region          string `json:"region" env:"AWS_REGION"`
	Endpoint        string `json:"endpoint" env:"S3_ENDPOINT"` // For MinIO
	AccessKeyID     string `json:"access_key_id" env:"AWS_ACCESS_KEY_ID"`
	SecretAccessKey string `json:"secret_access_key" env:"AWS_SECRET_ACCESS_KEY"`
	Bucket          string `json:"bucket" env:"S3_BUCKET"`
	UsePathStyle    bool   `json:"use_path_style" env:"S3_PATH_STYLE"` // Required for MinIO
}

// RAGConfig contains RAG/embedding configuration.
type RAGConfig struct {
	Enabled          bool            `json:"enabled" env:"FEATURE_RAG"`
	Embedding        EmbeddingConfig `json:"embedding"`
	Chunking         ChunkingConfig  `json:"chunking"`
	DefaultNamespace string          `json:"default_namespace"`
}

// EmbeddingConfig contains embedding model configuration.
type EmbeddingConfig struct {
	Provider   string `json:"provider" env:"EMBEDDING_PROVIDER"` // openai, cohere, local
	Model      string `json:"model" env:"EMBEDDING_MODEL"`
	APIKey     string `json:"api_key" env:"EMBEDDING_API_KEY"`
	Endpoint   string `json:"endpoint" env:"EMBEDDING_ENDPOINT"` // For local/custom
	Dimensions int    `json:"dimensions"`
	BatchSize  int    `json:"batch_size"`
}

// ChunkingConfig contains document chunking configuration.
type ChunkingConfig struct {
	Strategy     string `json:"strategy"` // fixed, sentence, paragraph, semantic, code, markdown
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	MinChunkSize int    `json:"min_chunk_size"`
	MaxChunkSize int    `json:"max_chunk_size"`
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
	Orchestrator AgentConfig `json:"orchestrator" yaml:"orchestrator"`
	Planner      AgentConfig `json:"planner" yaml:"planner"`
	Researcher   AgentConfig `json:"researcher" yaml:"researcher"`
	Coder        AgentConfig `json:"coder" yaml:"coder"`
	Reviewer     AgentConfig `json:"reviewer" yaml:"reviewer"`
	Executor     AgentConfig `json:"executor" yaml:"executor"`
}

// AgentConfig contains configuration for a single agent.
type AgentConfig struct {
	Name         string   `json:"name" yaml:"name"`
	Type         string   `json:"type" yaml:"type"`
	Enabled      bool     `json:"enabled" yaml:"enabled"`
	Model        string   `json:"model" yaml:"model"`
	MaxTokens    int      `json:"max_tokens" yaml:"max_tokens"`
	Temperature  float64  `json:"temperature" yaml:"temperature"`
	SystemPrompt string   `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	Skills       []string `json:"skills,omitempty" yaml:"skills,omitempty"` // References to skill names in Skills map
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
			Provider:     "api", // Default to API; set to "claude_code" to use subscription
			Model:        "claude-sonnet-4-20250514",
			MaxTokens:    4096,
			Temperature:  0.7,
			WorkingDir:   ".",
			AllowedTools: []string{"Read", "Write", "Bash", "Glob", "Grep"},
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
		Logging: LoggingConfig{
			Level:         "info",
			Format:        "json",
			Output:        "stdout",
			AddCaller:     true,
			AddStacktrace: false,
			Development:   false,
			Sampling: &LogSamplingConfig{
				Enabled:    false,
				Initial:    100,
				Thereafter: 100,
			},
		},
		Metrics: MetricsConfig{
			Enabled:              true,
			Address:              ":9090",
			Path:                 "/metrics",
			Namespace:            "claude_orchestrator",
			EnableGoMetrics:      true,
			EnableProcessMetrics: true,
		},
		Storage: StorageConfig{
			Enabled: true,
			Type:    "local",
			Local: LocalStorageConfig{
				BasePath:    "./storage",
				MaxFileSize: 100 * 1024 * 1024, // 100MB
			},
			S3: S3StorageConfig{
				Region:       "us-east-1",
				UsePathStyle: false,
			},
		},
		RAG: RAGConfig{
			Enabled:          true,
			DefaultNamespace: "default",
			Embedding: EmbeddingConfig{
				Provider:   "openai",
				Model:      "text-embedding-3-small",
				Dimensions: 1536,
				BatchSize:  100,
			},
			Chunking: ChunkingConfig{
				Strategy:     "sentence",
				ChunkSize:    1000,
				ChunkOverlap: 200,
				MinChunkSize: 100,
				MaxChunkSize: 2000,
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
		MCP: MCPConfig{
			Enabled: false,
			GitLab: GitLabMCPConfig{
				Enabled: false,
				BaseURL: "https://gitlab.com",
			},
			Database: DatabaseMCPConfig{
				Enabled:  false,
				Driver:   "postgres",
				Host:     "localhost",
				Port:     5432,
				SSLMode:  "disable",
				MaxRows:  100,
				ReadOnly: true,
			},
		},
		LSP: LSPConfig{
			Enabled:   false,
			Address:   "localhost:9999",
			Languages: []string{"go"},
			Features: LSPFeatures{
				Hover:       true,
				Completion:  true,
				Definition:  true,
				References:  true,
				Symbols:     true,
				Diagnostics: true,
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
// Supports both JSON (.json) and YAML (.yaml, .yml) formats.
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	// Load from file if provided
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse YAML config file: %w", err)
			}
		case ".json":
			if err := json.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse JSON config file: %w", err)
			}
		default:
			// Try YAML first, then JSON
			if err := yaml.Unmarshal(data, config); err != nil {
				if err := json.Unmarshal(data, config); err != nil {
					return nil, fmt.Errorf("failed to parse config file (tried YAML and JSON): %w", err)
				}
			}
		}
	}

	// Override with environment variables
	applyEnvironmentOverrides(config)

	return config, nil
}

// LoadConfigWithDefaults loads config and applies default skills if none defined.
func LoadConfigWithDefaults(path string) (*Config, error) {
	config, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	// Apply default skills if none defined
	if config.Skills == nil {
		config.Skills = DefaultSkills()
	}

	return config, nil
}

// DefaultSkills returns the default skill configurations.
func DefaultSkills() map[string]SkillConfig {
	return map[string]SkillConfig{
		"code_generation": {
			Name:        "Code Generation",
			Description: "Generate code from specifications",
			Tools:       []string{"Read", "Write", "Edit"},
			Enabled:     true,
			Prompts: SkillPrompts{
				Before: "Analyze requirements before generating code.",
				After:  "Verify the code compiles and follows best practices.",
			},
		},
		"refactoring": {
			Name:        "Code Refactoring",
			Description: "Improve existing code structure",
			Tools:       []string{"Read", "Write", "Edit", "Grep"},
			Enabled:     true,
			Prompts: SkillPrompts{
				Before: "Analyze the existing code structure.",
				After:  "Ensure refactored code maintains functionality.",
			},
		},
		"bug_fixing": {
			Name:        "Bug Fixing",
			Description: "Identify and fix bugs",
			Tools:       []string{"Read", "Write", "Edit", "Bash", "Grep"},
			Enabled:     true,
			Prompts: SkillPrompts{
				Before: "Reproduce the bug to understand the issue.",
				After:  "Write a test to prevent regression.",
			},
		},
		"test_writing": {
			Name:        "Test Writing",
			Description: "Write unit and integration tests",
			Tools:       []string{"Read", "Write", "Edit", "Bash"},
			Enabled:     true,
			Prompts: SkillPrompts{
				Before: "Identify edge cases to test.",
				After:  "Run tests to verify they pass.",
			},
		},
		"code_review": {
			Name:        "Code Review",
			Description: "Review code for quality and issues",
			Tools:       []string{"Read", "Grep"},
			Enabled:     true,
			Prompts: SkillPrompts{
				Before: "Understand the context and requirements.",
				After:  "Provide actionable feedback.",
			},
		},
		"security_audit": {
			Name:        "Security Audit",
			Description: "Check for security vulnerabilities",
			Tools:       []string{"Read", "Grep"},
			Enabled:     true,
			Prompts: SkillPrompts{
				Before: "Focus on common vulnerability patterns.",
				After:  "Prioritize findings by severity.",
			},
		},
	}
}

// GetAgentSkills returns the skill configurations for an agent.
func (c *Config) GetAgentSkills(agentType string) []SkillConfig {
	var agentConfig *AgentConfig

	switch agentType {
	case "orchestrator":
		agentConfig = &c.Agents.Orchestrator
	case "planner":
		agentConfig = &c.Agents.Planner
	case "researcher":
		agentConfig = &c.Agents.Researcher
	case "coder":
		agentConfig = &c.Agents.Coder
	case "reviewer":
		agentConfig = &c.Agents.Reviewer
	case "executor":
		agentConfig = &c.Agents.Executor
	default:
		return nil
	}

	if agentConfig == nil || len(agentConfig.Skills) == 0 {
		return nil
	}

	var skills []SkillConfig
	for _, skillName := range agentConfig.Skills {
		if skill, ok := c.Skills[skillName]; ok && skill.Enabled {
			skills = append(skills, skill)
		}
	}

	return skills
}

// SaveConfig saves configuration to a file.
// Format is determined by file extension (.yaml, .yml for YAML, .json for JSON).
func SaveConfig(config *Config, path string) error {
	var data []byte
	var err error

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config to YAML: %w", err)
		}
	default:
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config to JSON: %w", err)
		}
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
	if v := os.Getenv("CLAUDE_PROVIDER"); v != "" {
		config.Claude.Provider = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		config.Claude.APIKey = v
	}
	if v := os.Getenv("CLAUDE_MODEL"); v != "" {
		config.Claude.Model = v
	}
	if v := os.Getenv("WORKING_DIR"); v != "" {
		config.Claude.WorkingDir = v
	}
	if v := os.Getenv("CLAUDE_ALLOWED_TOOLS"); v != "" {
		config.Claude.AllowedTools = strings.Split(v, ",")
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

	// Storage
	if v := os.Getenv("FEATURE_STORAGE"); v != "" {
		config.Storage.Enabled = parseBool(v)
	}
	if v := os.Getenv("STORAGE_TYPE"); v != "" {
		config.Storage.Type = v
	}
	if v := os.Getenv("STORAGE_LOCAL_PATH"); v != "" {
		config.Storage.Local.BasePath = v
	}
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		config.Storage.S3.Endpoint = v
	}
	if v := os.Getenv("AWS_REGION"); v != "" {
		config.Storage.S3.Region = v
	}
	if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
		config.Storage.S3.AccessKeyID = v
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
		config.Storage.S3.SecretAccessKey = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		config.Storage.S3.Bucket = v
	}
	if v := os.Getenv("S3_PATH_STYLE"); v != "" {
		config.Storage.S3.UsePathStyle = parseBool(v)
	}

	// RAG
	if v := os.Getenv("FEATURE_RAG"); v != "" {
		config.RAG.Enabled = parseBool(v)
	}
	if v := os.Getenv("EMBEDDING_PROVIDER"); v != "" {
		config.RAG.Embedding.Provider = v
	}
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		config.RAG.Embedding.Model = v
	}
	if v := os.Getenv("EMBEDDING_API_KEY"); v != "" {
		config.RAG.Embedding.APIKey = v
	}
	if v := os.Getenv("EMBEDDING_ENDPOINT"); v != "" {
		config.RAG.Embedding.Endpoint = v
	}

	// Logging
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Logging.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		config.Logging.Format = v
	}
	if v := os.Getenv("LOG_OUTPUT"); v != "" {
		config.Logging.Output = v
	}
	if v := os.Getenv("LOG_DEVELOPMENT"); v != "" {
		config.Logging.Development = parseBool(v)
	}

	// Metrics
	if v := os.Getenv("METRICS_ENABLED"); v != "" {
		config.Metrics.Enabled = parseBool(v)
	}
	if v := os.Getenv("METRICS_ADDRESS"); v != "" {
		config.Metrics.Address = v
	}
	if v := os.Getenv("METRICS_PATH"); v != "" {
		config.Metrics.Path = v
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate Claude configuration based on provider
	switch c.Claude.Provider {
	case "api", "":
		// API provider requires an API key
		if c.Claude.APIKey == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY is required when using 'api' provider")
		}
	case "claude_code":
		// Claude Code provider uses subscription auth, no API key needed
		// The CLI will handle authentication
	default:
		return fmt.Errorf("invalid Claude provider: %s (must be 'api' or 'claude_code')", c.Claude.Provider)
	}

	if c.Temporal.Address == "" {
		return fmt.Errorf("Temporal address is required")
	}

	if c.Workflow.MaxAgents <= 0 {
		return fmt.Errorf("max_agents must be positive")
	}

	return nil
}

// IsClaudeCodeProvider returns true if using Claude Code provider.
func (c *Config) IsClaudeCodeProvider() bool {
	return c.Claude.Provider == "claude_code"
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
	case "storage", "file_storage":
		return c.Storage.Enabled
	case "rag", "embeddings":
		return c.RAG.Enabled
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
