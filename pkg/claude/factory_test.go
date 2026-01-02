package claude

import (
	"testing"
)

func TestClientConfig_Defaults(t *testing.T) {
	cfg := ClientConfig{}

	// Empty config should have zero values
	if cfg.Provider != "" {
		t.Errorf("Expected empty Provider, got '%s'", cfg.Provider)
	}
	if cfg.APIKey != "" {
		t.Error("Expected empty APIKey")
	}
	if cfg.WorkingDir != "" {
		t.Error("Expected empty WorkingDir")
	}
	if cfg.Model != "" {
		t.Error("Expected empty Model")
	}
}

func TestNewClientFromConfig_APIProvider(t *testing.T) {
	cfg := ClientConfig{
		Provider: ProviderAPI,
		APIKey:   "test-api-key",
	}

	client, err := NewClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientFromConfig failed: %v", err)
	}

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.Provider() != ProviderAPI {
		t.Errorf("Expected ProviderAPI, got %s", client.Provider())
	}
}

func TestNewClientFromConfig_APIProvider_NoKey(t *testing.T) {
	cfg := ClientConfig{
		Provider: ProviderAPI,
		APIKey:   "", // No key
	}

	_, err := NewClientFromConfig(cfg)
	if err == nil {
		t.Error("Expected error for API provider without key")
	}
}

func TestNewClientFromConfig_EmptyProvider_WithKey(t *testing.T) {
	// Empty provider should default to API
	cfg := ClientConfig{
		Provider: "",
		APIKey:   "test-key",
	}

	client, err := NewClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientFromConfig failed: %v", err)
	}

	if client.Provider() != ProviderAPI {
		t.Errorf("Expected ProviderAPI for empty provider, got %s", client.Provider())
	}
}

func TestNewClientFromConfig_EmptyProvider_NoKey(t *testing.T) {
	// Empty provider with no key should fail (defaults to API which needs key)
	cfg := ClientConfig{
		Provider: "",
		APIKey:   "",
	}

	_, err := NewClientFromConfig(cfg)
	if err == nil {
		t.Error("Expected error for empty provider without key")
	}
}

func TestNewClientFromConfig_UnknownProvider(t *testing.T) {
	cfg := ClientConfig{
		Provider: "unknown_provider",
	}

	_, err := NewClientFromConfig(cfg)
	if err == nil {
		t.Error("Expected error for unknown provider")
	}
}

func TestNewClientFromConfig_ClaudeCodeProvider_Options(t *testing.T) {
	// Note: This test only validates that the config is passed correctly
	// The actual client creation will fail if claude CLI is not installed
	cfg := ClientConfig{
		Provider:     ProviderClaudeCode,
		Model:        "claude-opus-4-20250514",
		WorkingDir:   "/custom/dir",
		AllowedTools: []string{"Read", "Write"},
	}

	// This may fail if claude CLI is not installed, which is expected
	client, err := NewClientFromConfig(cfg)
	if err != nil {
		// Expected if CLI is not installed
		t.Skipf("Claude Code CLI not available: %v", err)
		return
	}

	if client.Provider() != ProviderClaudeCode {
		t.Errorf("Expected ProviderClaudeCode, got %s", client.Provider())
	}
}

func TestMustNewClient_Success(t *testing.T) {
	cfg := ClientConfig{
		Provider: ProviderAPI,
		APIKey:   "test-key",
	}

	// Should not panic
	client := MustNewClient(cfg)
	if client == nil {
		t.Error("Expected non-nil client")
	}
}

func TestMustNewClient_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for invalid config")
		}
	}()

	cfg := ClientConfig{
		Provider: ProviderAPI,
		APIKey:   "", // No key - should panic
	}

	MustNewClient(cfg)
}

func TestAutoDetectProvider_ReturnsProvider(t *testing.T) {
	// This test just ensures the function runs without error
	// The actual result depends on whether claude CLI is installed
	provider, err := AutoDetectProvider()
	if err != nil {
		t.Fatalf("AutoDetectProvider failed: %v", err)
	}

	// Should return either API or ClaudeCode
	if provider != ProviderAPI && provider != ProviderClaudeCode {
		t.Errorf("Unexpected provider: %s", provider)
	}
}

func TestProviderConstants(t *testing.T) {
	// Verify provider constant values
	if ProviderAPI != "api" {
		t.Errorf("ProviderAPI should be 'api', got '%s'", ProviderAPI)
	}
	if ProviderClaudeCode != "claude_code" {
		t.Errorf("ProviderClaudeCode should be 'claude_code', got '%s'", ProviderClaudeCode)
	}
}

func TestClientConfig_AllFields(t *testing.T) {
	cfg := ClientConfig{
		Provider:     ProviderClaudeCode,
		APIKey:       "api-key",
		WorkingDir:   "/work",
		AllowedTools: []string{"Read", "Write", "Bash"},
		Model:        "custom-model",
	}

	if cfg.Provider != ProviderClaudeCode {
		t.Errorf("Provider mismatch: got %s", cfg.Provider)
	}
	if cfg.APIKey != "api-key" {
		t.Errorf("APIKey mismatch")
	}
	if cfg.WorkingDir != "/work" {
		t.Errorf("WorkingDir mismatch")
	}
	if len(cfg.AllowedTools) != 3 {
		t.Errorf("AllowedTools length mismatch")
	}
	if cfg.Model != "custom-model" {
		t.Errorf("Model mismatch")
	}
}
