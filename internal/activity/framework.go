package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/memory"
	"github.com/anthropics/claude-orchestrator/pkg/claude"
	"go.temporal.io/sdk/activity"
)

// FrameworkActivities contains activities for framework learning.
type FrameworkActivities struct {
	claudeClient *claude.Client
	store        memory.Store
}

// NewFrameworkActivities creates a new FrameworkActivities instance.
func NewFrameworkActivities(apiKey string, store memory.Store) (*FrameworkActivities, error) {
	client, err := claude.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Claude client: %w", err)
	}
	return &FrameworkActivities{
		claudeClient: client,
		store:        store,
	}, nil
}

// FrameworkKnowledge represents learned knowledge about a framework.
type FrameworkKnowledge struct {
	Name           string                 `json:"name"`
	Language       string                 `json:"language"`
	Version        string                 `json:"version,omitempty"`
	Description    string                 `json:"description"`
	LearnedAt      time.Time              `json:"learned_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Patterns       []FrameworkPattern     `json:"patterns"`
	BestPractices  []string               `json:"best_practices"`
	CommonMistakes []string               `json:"common_mistakes"`
	Dependencies   []string               `json:"dependencies"`
	FileStructure  []FilePattern          `json:"file_structure"`
	CodeExamples   []CodeExample          `json:"code_examples"`
	Concepts       []FrameworkConcept     `json:"concepts"`
	APIReference   map[string]APIEndpoint `json:"api_reference,omitempty"`
	Configuration  map[string]interface{} `json:"configuration,omitempty"`
	Sources        []KnowledgeSource      `json:"sources"`
}

// FrameworkPattern represents a design pattern used in the framework.
type FrameworkPattern struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	UseCase     string `json:"use_case"`
	Example     string `json:"example,omitempty"`
}

// FilePattern represents a common file structure pattern.
type FilePattern struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Template    string `json:"template,omitempty"`
}

// CodeExample represents an example code snippet.
type CodeExample struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Code        string   `json:"code"`
	Language    string   `json:"language"`
	Tags        []string `json:"tags"`
}

// FrameworkConcept represents a core concept of the framework.
type FrameworkConcept struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Related     []string `json:"related,omitempty"`
}

// APIEndpoint represents an API endpoint or function.
type APIEndpoint struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]string      `json:"parameters,omitempty"`
	Returns     string                 `json:"returns,omitempty"`
	Example     string                 `json:"example,omitempty"`
}

// KnowledgeSource represents a source of framework knowledge.
type KnowledgeSource struct {
	Type       string    `json:"type"` // documentation, codebase, example, web
	Path       string    `json:"path"`
	LearnedAt  time.Time `json:"learned_at"`
	TokensUsed int       `json:"tokens_used,omitempty"`
}

// LearnFrameworkRequest represents a request to learn a framework.
type LearnFrameworkRequest struct {
	Name           string   `json:"name"`
	Language       string   `json:"language"`
	Version        string   `json:"version,omitempty"`
	DocumentPaths  []string `json:"document_paths,omitempty"`
	CodebasePath   string   `json:"codebase_path,omitempty"`
	WebURLs        []string `json:"web_urls,omitempty"`
	FocusAreas     []string `json:"focus_areas,omitempty"` // e.g., ["routing", "database", "auth"]
	MaxTokens      int      `json:"max_tokens,omitempty"`
}

// LearnFrameworkResult represents the result of learning a framework.
type LearnFrameworkResult struct {
	FrameworkID    string    `json:"framework_id"`
	Name           string    `json:"name"`
	ConceptsLearned int      `json:"concepts_learned"`
	PatternsLearned int      `json:"patterns_learned"`
	ExamplesLearned int      `json:"examples_learned"`
	SourcesUsed    int       `json:"sources_used"`
	TotalTokens    int       `json:"total_tokens"`
	LearnedAt      time.Time `json:"learned_at"`
}

// LearnFramework learns a framework from various sources.
func (a *FrameworkActivities) LearnFramework(ctx context.Context, req LearnFrameworkRequest) (*LearnFrameworkResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Learning framework", "name", req.Name, "language", req.Language)

	knowledge := &FrameworkKnowledge{
		Name:           req.Name,
		Language:       req.Language,
		Version:        req.Version,
		LearnedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Patterns:       []FrameworkPattern{},
		BestPractices:  []string{},
		CommonMistakes: []string{},
		Dependencies:   []string{},
		FileStructure:  []FilePattern{},
		CodeExamples:   []CodeExample{},
		Concepts:       []FrameworkConcept{},
		APIReference:   make(map[string]APIEndpoint),
		Configuration:  make(map[string]interface{}),
		Sources:        []KnowledgeSource{},
	}

	totalTokens := 0

	// Learn from documents
	if len(req.DocumentPaths) > 0 {
		activity.RecordHeartbeat(ctx, "learning from documents")
		tokens, err := a.learnFromDocuments(ctx, knowledge, req.DocumentPaths, req.FocusAreas)
		if err != nil {
			logger.Warn("Failed to learn from some documents", "error", err)
		}
		totalTokens += tokens
	}

	// Learn from codebase
	if req.CodebasePath != "" {
		activity.RecordHeartbeat(ctx, "learning from codebase")
		tokens, err := a.learnFromCodebase(ctx, knowledge, req.CodebasePath, req.FocusAreas)
		if err != nil {
			logger.Warn("Failed to learn from codebase", "error", err)
		}
		totalTokens += tokens
	}

	// Generate comprehensive summary
	activity.RecordHeartbeat(ctx, "generating framework summary")
	if err := a.generateFrameworkSummary(ctx, knowledge); err != nil {
		logger.Warn("Failed to generate summary", "error", err)
	}

	// Store the knowledge
	frameworkID := fmt.Sprintf("%s-%s", strings.ToLower(req.Language), strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")))
	if err := a.storeFrameworkKnowledge(ctx, frameworkID, knowledge); err != nil {
		return nil, fmt.Errorf("failed to store framework knowledge: %w", err)
	}

	return &LearnFrameworkResult{
		FrameworkID:     frameworkID,
		Name:            req.Name,
		ConceptsLearned: len(knowledge.Concepts),
		PatternsLearned: len(knowledge.Patterns),
		ExamplesLearned: len(knowledge.CodeExamples),
		SourcesUsed:     len(knowledge.Sources),
		TotalTokens:     totalTokens,
		LearnedAt:       knowledge.LearnedAt,
	}, nil
}

// GetFrameworkKnowledgeRequest represents a request to get framework knowledge.
type GetFrameworkKnowledgeRequest struct {
	FrameworkID string `json:"framework_id"`
}

// GetFrameworkKnowledge retrieves stored framework knowledge.
func (a *FrameworkActivities) GetFrameworkKnowledge(ctx context.Context, req GetFrameworkKnowledgeRequest) (*FrameworkKnowledge, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting framework knowledge", "id", req.FrameworkID)

	key := frameworkKey(req.FrameworkID)
	mem, err := a.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get framework knowledge: %w", err)
	}

	var knowledge FrameworkKnowledge
	if err := json.Unmarshal([]byte(mem.Value), &knowledge); err != nil {
		return nil, fmt.Errorf("failed to parse framework knowledge: %w", err)
	}

	return &knowledge, nil
}

// GenerateFrameworkPromptRequest represents a request to generate a prompt with framework knowledge.
type GenerateFrameworkPromptRequest struct {
	FrameworkID string   `json:"framework_id"`
	Task        string   `json:"task"`
	FocusAreas  []string `json:"focus_areas,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
}

// GenerateFrameworkPromptResult contains the generated prompt.
type GenerateFrameworkPromptResult struct {
	SystemPrompt    string           `json:"system_prompt"`
	ContextPrompt   string           `json:"context_prompt"`
	RelevantPatterns []FrameworkPattern `json:"relevant_patterns"`
	RelevantExamples []CodeExample    `json:"relevant_examples"`
}

// GenerateFrameworkPrompt generates a prompt enriched with framework knowledge.
func (a *FrameworkActivities) GenerateFrameworkPrompt(ctx context.Context, req GenerateFrameworkPromptRequest) (*GenerateFrameworkPromptResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Generating framework prompt", "id", req.FrameworkID, "task", req.Task)

	knowledge, err := a.GetFrameworkKnowledge(ctx, GetFrameworkKnowledgeRequest{FrameworkID: req.FrameworkID})
	if err != nil {
		return nil, err
	}

	result := &GenerateFrameworkPromptResult{
		RelevantPatterns: []FrameworkPattern{},
		RelevantExamples: []CodeExample{},
	}

	// Find relevant patterns and examples based on task and focus areas
	taskLower := strings.ToLower(req.Task)
	for _, pattern := range knowledge.Patterns {
		if containsAny(strings.ToLower(pattern.Name+" "+pattern.Description), req.FocusAreas) ||
			strings.Contains(taskLower, strings.ToLower(pattern.Name)) {
			result.RelevantPatterns = append(result.RelevantPatterns, pattern)
		}
	}

	for _, example := range knowledge.CodeExamples {
		if containsAny(strings.ToLower(example.Title+" "+example.Description), req.FocusAreas) ||
			hasAnyTag(example.Tags, req.FocusAreas) {
			result.RelevantExamples = append(result.RelevantExamples, example)
		}
	}

	// Build system prompt
	var systemPrompt strings.Builder
	systemPrompt.WriteString(fmt.Sprintf("You are an expert %s developer specializing in %s", knowledge.Language, knowledge.Name))
	if knowledge.Version != "" {
		systemPrompt.WriteString(fmt.Sprintf(" (version %s)", knowledge.Version))
	}
	systemPrompt.WriteString(".\n\n")

	systemPrompt.WriteString("Framework Knowledge:\n")
	systemPrompt.WriteString(fmt.Sprintf("- Description: %s\n", knowledge.Description))

	if len(knowledge.BestPractices) > 0 {
		systemPrompt.WriteString("\nBest Practices:\n")
		for _, bp := range knowledge.BestPractices {
			systemPrompt.WriteString(fmt.Sprintf("- %s\n", bp))
		}
	}

	if len(knowledge.CommonMistakes) > 0 {
		systemPrompt.WriteString("\nCommon Mistakes to Avoid:\n")
		for _, cm := range knowledge.CommonMistakes {
			systemPrompt.WriteString(fmt.Sprintf("- %s\n", cm))
		}
	}

	result.SystemPrompt = systemPrompt.String()

	// Build context prompt with relevant patterns and examples
	var contextPrompt strings.Builder
	contextPrompt.WriteString("Relevant Framework Context:\n\n")

	if len(result.RelevantPatterns) > 0 {
		contextPrompt.WriteString("Applicable Patterns:\n")
		for _, p := range result.RelevantPatterns {
			contextPrompt.WriteString(fmt.Sprintf("- %s: %s\n", p.Name, p.Description))
			if p.Example != "" {
				contextPrompt.WriteString(fmt.Sprintf("  Example: %s\n", p.Example))
			}
		}
		contextPrompt.WriteString("\n")
	}

	if len(result.RelevantExamples) > 0 {
		contextPrompt.WriteString("Reference Examples:\n")
		for _, e := range result.RelevantExamples {
			contextPrompt.WriteString(fmt.Sprintf("--- %s ---\n", e.Title))
			contextPrompt.WriteString(fmt.Sprintf("%s\n```%s\n%s\n```\n\n", e.Description, e.Language, e.Code))
		}
	}

	result.ContextPrompt = contextPrompt.String()

	return result, nil
}

// ListFrameworksRequest represents a request to list learned frameworks.
type ListFrameworksRequest struct {
	Language string `json:"language,omitempty"`
}

// FrameworkSummary provides a brief summary of a learned framework.
type FrameworkSummary struct {
	FrameworkID    string    `json:"framework_id"`
	Name           string    `json:"name"`
	Language       string    `json:"language"`
	Version        string    `json:"version,omitempty"`
	Description    string    `json:"description"`
	ConceptCount   int       `json:"concept_count"`
	PatternCount   int       `json:"pattern_count"`
	ExampleCount   int       `json:"example_count"`
	LearnedAt      time.Time `json:"learned_at"`
}

// ListFrameworksResult contains the list of learned frameworks.
type ListFrameworksResult struct {
	Frameworks []FrameworkSummary `json:"frameworks"`
	Total      int                `json:"total"`
}

// ListFrameworks lists all learned frameworks.
func (a *FrameworkActivities) ListFrameworks(ctx context.Context, req ListFrameworksRequest) (*ListFrameworksResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing frameworks", "language", req.Language)

	pattern := "frameworks:*"
	keys, err := a.store.List(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list frameworks: %w", err)
	}

	result := &ListFrameworksResult{
		Frameworks: []FrameworkSummary{},
	}

	for _, key := range keys {
		if strings.HasSuffix(key, ":index") {
			continue
		}

		mem, err := a.store.Get(ctx, key)
		if err != nil {
			continue
		}

		var knowledge FrameworkKnowledge
		if err := json.Unmarshal([]byte(mem.Value), &knowledge); err != nil {
			continue
		}

		// Filter by language if specified
		if req.Language != "" && !strings.EqualFold(knowledge.Language, req.Language) {
			continue
		}

		frameworkID := strings.TrimPrefix(key, "frameworks:")
		result.Frameworks = append(result.Frameworks, FrameworkSummary{
			FrameworkID:  frameworkID,
			Name:         knowledge.Name,
			Language:     knowledge.Language,
			Version:      knowledge.Version,
			Description:  knowledge.Description,
			ConceptCount: len(knowledge.Concepts),
			PatternCount: len(knowledge.Patterns),
			ExampleCount: len(knowledge.CodeExamples),
			LearnedAt:    knowledge.LearnedAt,
		})
	}

	result.Total = len(result.Frameworks)
	return result, nil
}

func (a *FrameworkActivities) learnFromDocuments(ctx context.Context, knowledge *FrameworkKnowledge, paths []string, focusAreas []string) (int, error) {
	totalTokens := 0

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		prompt := fmt.Sprintf(`Analyze this %s framework documentation and extract:

1. Core concepts and their descriptions
2. Design patterns used
3. Best practices
4. Common mistakes to avoid
5. Code examples with explanations
6. File structure patterns

Focus areas: %s

Documentation content:
%s

Respond with JSON:
{
  "concepts": [{"name": "...", "description": "...", "related": ["..."]}],
  "patterns": [{"name": "...", "description": "...", "use_case": "...", "example": "..."}],
  "best_practices": ["..."],
  "common_mistakes": ["..."],
  "code_examples": [{"title": "...", "description": "...", "code": "...", "language": "...", "tags": ["..."]}],
  "file_structure": [{"path": "...", "description": "...", "required": true/false}]
}`, knowledge.Name, strings.Join(focusAreas, ", "), string(content))

		response, err := a.claudeClient.SimpleCompletion(ctx, frameworkLearningSystemPrompt, prompt)
		if err != nil {
			continue
		}

		// Parse and merge knowledge
		parsed := parseFrameworkLearning(response)
		mergeFrameworkKnowledge(knowledge, parsed)

		knowledge.Sources = append(knowledge.Sources, KnowledgeSource{
			Type:      "documentation",
			Path:      path,
			LearnedAt: time.Now(),
		})

		totalTokens += len(content)/4 + len(response)/4 // Rough estimate
	}

	return totalTokens, nil
}

func (a *FrameworkActivities) learnFromCodebase(ctx context.Context, knowledge *FrameworkKnowledge, path string, focusAreas []string) (int, error) {
	totalTokens := 0

	// Collect relevant code files
	var codeFiles []string
	extensions := getLanguageExtensions(knowledge.Language)

	filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip common non-source directories
		if strings.Contains(filePath, "node_modules") ||
			strings.Contains(filePath, "vendor") ||
			strings.Contains(filePath, ".git") {
			return nil
		}

		ext := filepath.Ext(filePath)
		for _, e := range extensions {
			if ext == e {
				codeFiles = append(codeFiles, filePath)
				break
			}
		}
		return nil
	})

	// Analyze code files (limit to prevent token overflow)
	maxFiles := 20
	if len(codeFiles) > maxFiles {
		codeFiles = codeFiles[:maxFiles]
	}

	var codeContent strings.Builder
	for _, file := range codeFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		codeContent.WriteString(fmt.Sprintf("\n=== File: %s ===\n", file))
		codeContent.WriteString(string(content))
		codeContent.WriteString("\n")
	}

	if codeContent.Len() > 0 {
		prompt := fmt.Sprintf(`Analyze this %s codebase using the %s framework and extract:

1. Patterns and practices used
2. Code organization patterns
3. Example implementations
4. Configuration approaches

Focus areas: %s

Codebase:
%s

Respond with JSON:
{
  "patterns": [{"name": "...", "description": "...", "use_case": "...", "example": "..."}],
  "code_examples": [{"title": "...", "description": "...", "code": "...", "language": "%s", "tags": ["..."]}],
  "file_structure": [{"path": "...", "description": "...", "required": true/false}],
  "configuration": {"key": "value"}
}`, knowledge.Language, knowledge.Name, strings.Join(focusAreas, ", "), codeContent.String(), knowledge.Language)

		response, err := a.claudeClient.SimpleCompletion(ctx, frameworkLearningSystemPrompt, prompt)
		if err == nil {
			parsed := parseFrameworkLearning(response)
			mergeFrameworkKnowledge(knowledge, parsed)

			knowledge.Sources = append(knowledge.Sources, KnowledgeSource{
				Type:      "codebase",
				Path:      path,
				LearnedAt: time.Now(),
			})

			totalTokens += codeContent.Len()/4 + len(response)/4
		}
	}

	return totalTokens, nil
}

func (a *FrameworkActivities) generateFrameworkSummary(ctx context.Context, knowledge *FrameworkKnowledge) error {
	prompt := fmt.Sprintf(`Based on the following framework knowledge, generate a comprehensive description:

Framework: %s
Language: %s
Concepts: %d
Patterns: %d
Examples: %d

Concepts: %v
Patterns: %v
Best Practices: %v

Generate a 2-3 paragraph description that covers:
1. What the framework is and its main purpose
2. Key concepts and patterns
3. When and why to use it`,
		knowledge.Name,
		knowledge.Language,
		len(knowledge.Concepts),
		len(knowledge.Patterns),
		len(knowledge.CodeExamples),
		knowledge.Concepts,
		knowledge.Patterns,
		knowledge.BestPractices,
	)

	response, err := a.claudeClient.SimpleCompletion(ctx, "You are a technical writer. Generate clear, concise framework descriptions.", prompt)
	if err != nil {
		return err
	}

	knowledge.Description = response
	return nil
}

func (a *FrameworkActivities) storeFrameworkKnowledge(ctx context.Context, frameworkID string, knowledge *FrameworkKnowledge) error {
	data, err := json.Marshal(knowledge)
	if err != nil {
		return err
	}

	key := frameworkKey(frameworkID)
	opts := memory.StoreOptions{
		Namespace: "frameworks",
		Type:      memory.MemoryTypeLongTerm,
		Tags:      []string{knowledge.Language, knowledge.Name},
	}

	return a.store.Store(ctx, key, string(data), opts)
}

const frameworkLearningSystemPrompt = `You are a framework learning system. Your job is to:
1. Analyze framework documentation and code
2. Extract key concepts, patterns, and best practices
3. Identify code examples and their purposes
4. Understand file structure conventions
5. Document configuration options

Always respond with well-structured JSON. Be thorough but concise.`

func frameworkKey(frameworkID string) string {
	return fmt.Sprintf("frameworks:%s", frameworkID)
}

func getLanguageExtensions(language string) []string {
	switch strings.ToLower(language) {
	case "go", "golang":
		return []string{".go"}
	case "javascript", "js":
		return []string{".js", ".jsx", ".ts", ".tsx"}
	case "typescript", "ts":
		return []string{".ts", ".tsx"}
	case "python", "py":
		return []string{".py"}
	case "java":
		return []string{".java"}
	case "rust":
		return []string{".rs"}
	case "ruby":
		return []string{".rb"}
	case "php":
		return []string{".php"}
	case "csharp", "c#":
		return []string{".cs"}
	default:
		return []string{".go", ".js", ".ts", ".py", ".java", ".rs", ".rb"}
	}
}

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

func parseFrameworkLearning(response string) *FrameworkKnowledge {
	knowledge := &FrameworkKnowledge{
		Patterns:       []FrameworkPattern{},
		BestPractices:  []string{},
		CommonMistakes: []string{},
		FileStructure:  []FilePattern{},
		CodeExamples:   []CodeExample{},
		Concepts:       []FrameworkConcept{},
		Configuration:  make(map[string]interface{}),
	}

	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 {
		return knowledge
	}

	jsonStr := response[jsonStart : jsonEnd+1]

	var parsed struct {
		Concepts       []FrameworkConcept     `json:"concepts"`
		Patterns       []FrameworkPattern     `json:"patterns"`
		BestPractices  []string               `json:"best_practices"`
		CommonMistakes []string               `json:"common_mistakes"`
		CodeExamples   []CodeExample          `json:"code_examples"`
		FileStructure  []FilePattern          `json:"file_structure"`
		Configuration  map[string]interface{} `json:"configuration"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return knowledge
	}

	knowledge.Concepts = parsed.Concepts
	knowledge.Patterns = parsed.Patterns
	knowledge.BestPractices = parsed.BestPractices
	knowledge.CommonMistakes = parsed.CommonMistakes
	knowledge.CodeExamples = parsed.CodeExamples
	knowledge.FileStructure = parsed.FileStructure
	knowledge.Configuration = parsed.Configuration

	return knowledge
}

func mergeFrameworkKnowledge(target, source *FrameworkKnowledge) {
	target.Concepts = append(target.Concepts, source.Concepts...)
	target.Patterns = append(target.Patterns, source.Patterns...)
	target.BestPractices = append(target.BestPractices, source.BestPractices...)
	target.CommonMistakes = append(target.CommonMistakes, source.CommonMistakes...)
	target.CodeExamples = append(target.CodeExamples, source.CodeExamples...)
	target.FileStructure = append(target.FileStructure, source.FileStructure...)

	for k, v := range source.Configuration {
		target.Configuration[k] = v
	}
}
