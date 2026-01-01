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

	// Should fail without API key
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
