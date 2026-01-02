package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Check temporal defaults
	if cfg.Temporal.Address != "localhost:7233" {
		t.Errorf("Expected Temporal address 'localhost:7233', got '%s'", cfg.Temporal.Address)
	}
	if cfg.Temporal.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", cfg.Temporal.Namespace)
	}

	// Check feature defaults
	if !cfg.Features.DocumentAnalysis.Enabled {
		t.Error("Expected DocumentAnalysis to be enabled by default")
	}
	if !cfg.Features.RequirementsTracking.Enabled {
		t.Error("Expected RequirementsTracking to be enabled by default")
	}
	if !cfg.Features.FrameworkLearning.Enabled {
		t.Error("Expected FrameworkLearning to be enabled by default")
	}

	// Check agent defaults
	if !cfg.Agents.Coder.Enabled {
		t.Error("Expected Coder agent to be enabled by default")
	}
	if cfg.Agents.Coder.MaxTokens != 8192 {
		t.Errorf("Expected Coder MaxTokens 8192, got %d", cfg.Agents.Coder.MaxTokens)
	}
}

func TestIsFeatureEnabled(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		feature  string
		expected bool
	}{
		{"document_analysis", true},
		{"doc_analysis", true},
		{"requirements_tracking", true},
		{"req_tracking", true},
		{"framework_learning", true},
		{"code_review", true},
		{"parallel_execution", true},
		{"unknown_feature", false},
	}

	for _, tt := range tests {
		result := cfg.IsFeatureEnabled(tt.feature)
		if result != tt.expected {
			t.Errorf("IsFeatureEnabled(%s) = %v, want %v", tt.feature, result, tt.expected)
		}
	}
}

func TestGetAgentConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		agentType    string
		expectEnabled bool
	}{
		{"orchestrator", true},
		{"planner", true},
		{"researcher", true},
		{"coder", true},
		{"reviewer", true},
		{"executor", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		result := cfg.GetAgentConfig(tt.agentType)
		if result.Enabled != tt.expectEnabled {
			t.Errorf("GetAgentConfig(%s).Enabled = %v, want %v", tt.agentType, result.Enabled, tt.expectEnabled)
		}
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	// Save and restore environment
	originalAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	originalAddr := os.Getenv("TEMPORAL_ADDRESS")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", originalAPIKey)
		os.Setenv("TEMPORAL_ADDRESS", originalAddr)
	}()

	// Set test environment variables
	os.Setenv("ANTHROPIC_API_KEY", "test-api-key")
	os.Setenv("TEMPORAL_ADDRESS", "test-server:7233")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Claude.APIKey != "test-api-key" {
		t.Errorf("Expected API key 'test-api-key', got '%s'", cfg.Claude.APIKey)
	}

	if cfg.Temporal.Address != "test-server:7233" {
		t.Errorf("Expected Temporal address 'test-server:7233', got '%s'", cfg.Temporal.Address)
	}
}

func TestFeatureToggleEnvironment(t *testing.T) {
	// Save and restore environment
	originalDocAnalysis := os.Getenv("FEATURE_DOC_ANALYSIS")
	originalReqTracking := os.Getenv("FEATURE_REQ_TRACKING")
	defer func() {
		os.Setenv("FEATURE_DOC_ANALYSIS", originalDocAnalysis)
		os.Setenv("FEATURE_REQ_TRACKING", originalReqTracking)
	}()

	// Disable features via environment
	os.Setenv("FEATURE_DOC_ANALYSIS", "false")
	os.Setenv("FEATURE_REQ_TRACKING", "0")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Features.DocumentAnalysis.Enabled {
		t.Error("Expected DocumentAnalysis to be disabled via environment")
	}

	if cfg.Features.RequirementsTracking.Enabled {
		t.Error("Expected RequirementsTracking to be disabled via environment")
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"on", true},
		{"ON", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"off", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		result := parseBool(tt.input)
		if result != tt.expected {
			t.Errorf("parseBool(%s) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()

	// Should fail without API key when using default (api) provider
	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation to fail without API key")
	}

	// Should pass with API key
	cfg.Claude.APIKey = "test-key"
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass, got: %v", err)
	}

	// Should fail with invalid max agents
	cfg.Workflow.MaxAgents = 0
	err = cfg.Validate()
	if err == nil {
		t.Error("Expected validation to fail with zero max agents")
	}
}

func TestValidate_ClaudeCodeProvider(t *testing.T) {
	cfg := DefaultConfig()

	// Set provider to claude_code - should not require API key
	cfg.Claude.Provider = "claude_code"
	cfg.Claude.APIKey = "" // No API key

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass for claude_code provider without API key, got: %v", err)
	}
}

func TestValidate_APIProvider(t *testing.T) {
	cfg := DefaultConfig()

	// Explicit API provider without key should fail
	cfg.Claude.Provider = "api"
	cfg.Claude.APIKey = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation to fail for api provider without API key")
	}

	// With key should pass
	cfg.Claude.APIKey = "test-key"
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass for api provider with key, got: %v", err)
	}
}

func TestValidate_EmptyProvider(t *testing.T) {
	cfg := DefaultConfig()

	// Empty provider defaults to API behavior
	cfg.Claude.Provider = ""
	cfg.Claude.APIKey = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation to fail for empty provider without API key")
	}

	cfg.Claude.APIKey = "test-key"
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass for empty provider with key, got: %v", err)
	}
}

func TestValidate_InvalidProvider(t *testing.T) {
	cfg := DefaultConfig()

	cfg.Claude.Provider = "invalid_provider"
	cfg.Claude.APIKey = "test-key" // Even with key, invalid provider should fail

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation to fail for invalid provider")
	}
}

func TestIsClaudeCodeProvider(t *testing.T) {
	tests := []struct {
		provider string
		expected bool
	}{
		{"claude_code", true},
		{"api", false},
		{"", false},
		{"API", false}, // Case sensitive
		{"CLAUDE_CODE", false},
	}

	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.Claude.Provider = tt.provider

		result := cfg.IsClaudeCodeProvider()
		if result != tt.expected {
			t.Errorf("IsClaudeCodeProvider() for provider '%s' = %v, want %v", tt.provider, result, tt.expected)
		}
	}
}

func TestClaudeConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Check Claude config defaults
	if cfg.Claude.Provider != "api" {
		t.Errorf("Expected default provider 'api', got '%s'", cfg.Claude.Provider)
	}
	if cfg.Claude.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Expected default model 'claude-sonnet-4-20250514', got '%s'", cfg.Claude.Model)
	}
	if cfg.Claude.MaxTokens != 4096 {
		t.Errorf("Expected default MaxTokens 4096, got %d", cfg.Claude.MaxTokens)
	}
	if cfg.Claude.Temperature != 0.7 {
		t.Errorf("Expected default Temperature 0.7, got %f", cfg.Claude.Temperature)
	}
	if cfg.Claude.WorkingDir != "." {
		t.Errorf("Expected default WorkingDir '.', got '%s'", cfg.Claude.WorkingDir)
	}
	if len(cfg.Claude.AllowedTools) != 5 {
		t.Errorf("Expected 5 default AllowedTools, got %d", len(cfg.Claude.AllowedTools))
	}
}

func TestProviderEnvironmentOverride(t *testing.T) {
	// Save and restore environment
	originalProvider := os.Getenv("CLAUDE_PROVIDER")
	originalWorkingDir := os.Getenv("WORKING_DIR")
	originalTools := os.Getenv("CLAUDE_ALLOWED_TOOLS")
	defer func() {
		os.Setenv("CLAUDE_PROVIDER", originalProvider)
		os.Setenv("WORKING_DIR", originalWorkingDir)
		os.Setenv("CLAUDE_ALLOWED_TOOLS", originalTools)
	}()

	// Set test environment variables
	os.Setenv("CLAUDE_PROVIDER", "claude_code")
	os.Setenv("WORKING_DIR", "/custom/path")
	os.Setenv("CLAUDE_ALLOWED_TOOLS", "Read,Write,Bash")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Claude.Provider != "claude_code" {
		t.Errorf("Expected provider 'claude_code', got '%s'", cfg.Claude.Provider)
	}
	if cfg.Claude.WorkingDir != "/custom/path" {
		t.Errorf("Expected WorkingDir '/custom/path', got '%s'", cfg.Claude.WorkingDir)
	}
	if len(cfg.Claude.AllowedTools) != 3 {
		t.Errorf("Expected 3 AllowedTools, got %d", len(cfg.Claude.AllowedTools))
	}
	if cfg.Claude.AllowedTools[0] != "Read" {
		t.Errorf("Expected first tool 'Read', got '%s'", cfg.Claude.AllowedTools[0])
	}
}
