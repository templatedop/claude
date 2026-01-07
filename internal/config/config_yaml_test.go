package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_YAML(t *testing.T) {
	// Create a temporary YAML config file
	yamlContent := `
claude:
  provider: "claude_code"
  model: "claude-sonnet-4-20250514"
  working_dir: "/test/dir"
  allowed_tools:
    - "Read"
    - "Write"

temporal:
  address: "localhost:7233"
  namespace: "test-ns"
  task_queue: "test-queue"

agents:
  coder:
    name: "TestCoder"
    type: "coder"
    enabled: true
    model: "claude-sonnet-4-20250514"
    max_tokens: 8192
    temperature: 0.2
    skills:
      - "code_generation"
      - "test_writing"

skills:
  code_generation:
    name: "Code Generation"
    description: "Generate code from specifications"
    tools:
      - "Read"
      - "Write"
    enabled: true
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify YAML was parsed correctly
	if cfg.Claude.Provider != "claude_code" {
		t.Errorf("Expected provider 'claude_code', got '%s'", cfg.Claude.Provider)
	}

	if cfg.Claude.WorkingDir != "/test/dir" {
		t.Errorf("Expected working_dir '/test/dir', got '%s'", cfg.Claude.WorkingDir)
	}

	if cfg.Temporal.Namespace != "test-ns" {
		t.Errorf("Expected namespace 'test-ns', got '%s'", cfg.Temporal.Namespace)
	}

	if cfg.Agents.Coder.Name != "TestCoder" {
		t.Errorf("Expected coder name 'TestCoder', got '%s'", cfg.Agents.Coder.Name)
	}

	if len(cfg.Agents.Coder.Skills) != 2 {
		t.Errorf("Expected 2 skills for coder, got %d", len(cfg.Agents.Coder.Skills))
	}
}

func TestLoadConfig_JSON(t *testing.T) {
	jsonContent := `{
		"claude": {
			"provider": "api",
			"model": "claude-sonnet-4-20250514",
			"api_key": "test-key"
		},
		"temporal": {
			"address": "localhost:7233",
			"namespace": "json-ns"
		}
	}`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Claude.Provider != "api" {
		t.Errorf("Expected provider 'api', got '%s'", cfg.Claude.Provider)
	}

	if cfg.Temporal.Namespace != "json-ns" {
		t.Errorf("Expected namespace 'json-ns', got '%s'", cfg.Temporal.Namespace)
	}
}

func TestSaveConfig_YAML(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Claude.Provider = "claude_code"
	cfg.Temporal.Namespace = "save-test"

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "saved.yaml")

	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Reload and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if loaded.Claude.Provider != "claude_code" {
		t.Errorf("Expected provider 'claude_code', got '%s'", loaded.Claude.Provider)
	}

	if loaded.Temporal.Namespace != "save-test" {
		t.Errorf("Expected namespace 'save-test', got '%s'", loaded.Temporal.Namespace)
	}
}

func TestDefaultSkills(t *testing.T) {
	skills := DefaultSkills()

	expectedSkills := []string{
		"code_generation",
		"refactoring",
		"bug_fixing",
		"test_writing",
		"code_review",
		"security_audit",
	}

	for _, name := range expectedSkills {
		skill, ok := skills[name]
		if !ok {
			t.Errorf("Expected skill '%s' not found", name)
			continue
		}

		if skill.Name == "" {
			t.Errorf("Skill '%s' has empty name", name)
		}

		if len(skill.Tools) == 0 {
			t.Errorf("Skill '%s' has no tools", name)
		}

		if !skill.Enabled {
			t.Errorf("Skill '%s' should be enabled by default", name)
		}
	}
}

func TestGetAgentSkills(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Skills = DefaultSkills()
	cfg.Agents.Coder.Skills = []string{"code_generation", "test_writing"}

	skills := cfg.GetAgentSkills("coder")
	if len(skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(skills))
	}

	// Verify skill contents
	foundCodeGen := false
	foundTestWriting := false
	for _, skill := range skills {
		if skill.Name == "Code Generation" {
			foundCodeGen = true
		}
		if skill.Name == "Test Writing" {
			foundTestWriting = true
		}
	}

	if !foundCodeGen {
		t.Error("Expected to find 'Code Generation' skill")
	}
	if !foundTestWriting {
		t.Error("Expected to find 'Test Writing' skill")
	}
}

func TestGetAgentSkills_DisabledSkill(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Skills = DefaultSkills()

	// Disable code_generation skill
	skill := cfg.Skills["code_generation"]
	skill.Enabled = false
	cfg.Skills["code_generation"] = skill

	cfg.Agents.Coder.Skills = []string{"code_generation", "test_writing"}

	skills := cfg.GetAgentSkills("coder")
	// Should only return test_writing since code_generation is disabled
	if len(skills) != 1 {
		t.Errorf("Expected 1 skill (disabled skill should be excluded), got %d", len(skills))
	}
}

func TestGetAgentSkills_UnknownAgent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Skills = DefaultSkills()

	skills := cfg.GetAgentSkills("unknown_agent")
	if skills != nil {
		t.Error("Expected nil for unknown agent")
	}
}

func TestLoadConfigWithDefaults(t *testing.T) {
	cfg, err := LoadConfigWithDefaults("")
	if err != nil {
		t.Fatalf("LoadConfigWithDefaults failed: %v", err)
	}

	if cfg.Skills == nil {
		t.Error("Expected Skills to be populated with defaults")
	}

	if len(cfg.Skills) == 0 {
		t.Error("Expected at least one default skill")
	}
}

func TestSkillConfig_Fields(t *testing.T) {
	skill := SkillConfig{
		Name:        "Test Skill",
		Description: "A test skill",
		Tools:       []string{"Read", "Write"},
		Enabled:     true,
		Prompts: SkillPrompts{
			Before: "Before prompt",
			After:  "After prompt",
		},
		Commands: []string{"go test ./..."},
		Options: map[string]string{
			"key": "value",
		},
	}

	if skill.Name != "Test Skill" {
		t.Errorf("Expected name 'Test Skill', got '%s'", skill.Name)
	}

	if len(skill.Tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(skill.Tools))
	}

	if skill.Prompts.Before != "Before prompt" {
		t.Errorf("Expected before prompt, got '%s'", skill.Prompts.Before)
	}

	if len(skill.Commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(skill.Commands))
	}
}

func TestAgentConfig_WithSkills(t *testing.T) {
	agent := AgentConfig{
		Name:         "TestAgent",
		Type:         "coder",
		Enabled:      true,
		Model:        "claude-sonnet-4-20250514",
		MaxTokens:    8192,
		Temperature:  0.2,
		SystemPrompt: "You are a helpful assistant.",
		Skills:       []string{"code_generation", "bug_fixing"},
	}

	if agent.Name != "TestAgent" {
		t.Errorf("Expected name 'TestAgent', got '%s'", agent.Name)
	}

	if len(agent.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(agent.Skills))
	}
}
