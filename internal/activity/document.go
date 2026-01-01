package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/claude-orchestrator/pkg/claude"
	"go.temporal.io/sdk/activity"
)

// DocumentActivities contains activities for document analysis.
type DocumentActivities struct {
	claudeClient *claude.Client
}

// NewDocumentActivities creates a new DocumentActivities instance.
func NewDocumentActivities(apiKey string) (*DocumentActivities, error) {
	client, err := claude.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Claude client: %w", err)
	}
	return &DocumentActivities{claudeClient: client}, nil
}

// DocumentType represents the type of document being analyzed.
type DocumentType string

const (
	DocumentTypeRequirements  DocumentType = "requirements"
	DocumentTypeSpecification DocumentType = "specification"
	DocumentTypeDesign        DocumentType = "design"
	DocumentTypeUserStory     DocumentType = "user_story"
	DocumentTypeAPI           DocumentType = "api"
	DocumentTypeGeneral       DocumentType = "general"
)

// ReadDocumentRequest represents a request to read and analyze a document.
type ReadDocumentRequest struct {
	Path         string       `json:"path"`
	DocumentType DocumentType `json:"document_type,omitempty"`
	ExtractItems []string     `json:"extract_items,omitempty"` // What to extract: requirements, features, constraints, etc.
}

// ReadDocumentResult represents the result of document analysis.
type ReadDocumentResult struct {
	Path          string                 `json:"path"`
	Content       string                 `json:"content"`
	DocumentType  DocumentType           `json:"document_type"`
	Summary       string                 `json:"summary"`
	ExtractedData map[string]interface{} `json:"extracted_data"`
	Requirements  []Requirement          `json:"requirements,omitempty"`
	Metadata      DocumentMetadata       `json:"metadata"`
}

// DocumentMetadata contains metadata about the document.
type DocumentMetadata struct {
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	Extension  string `json:"extension"`
	LineCount  int    `json:"line_count"`
	WordCount  int    `json:"word_count"`
}

// Requirement represents a single requirement extracted from a document.
type Requirement struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Type        RequirementType   `json:"type"`
	Priority    RequirementPriority `json:"priority"`
	Status      RequirementStatus `json:"status"`
	Category    string            `json:"category,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Dependencies []string         `json:"dependencies,omitempty"`
	Acceptance  []string          `json:"acceptance_criteria,omitempty"`
	Source      string            `json:"source"`
	LineNumber  int               `json:"line_number,omitempty"`
}

// RequirementType represents the type of requirement.
type RequirementType string

const (
	RequirementTypeFunctional    RequirementType = "functional"
	RequirementTypeNonFunctional RequirementType = "non_functional"
	RequirementTypeTechnical     RequirementType = "technical"
	RequirementTypeBusiness      RequirementType = "business"
	RequirementTypeUser          RequirementType = "user"
	RequirementTypeSystem        RequirementType = "system"
	RequirementTypeConstraint    RequirementType = "constraint"
)

// RequirementPriority represents the priority of a requirement.
type RequirementPriority string

const (
	RequirementPriorityMust   RequirementPriority = "must"
	RequirementPriorityShould RequirementPriority = "should"
	RequirementPriorityCould  RequirementPriority = "could"
	RequirementPriorityWont   RequirementPriority = "wont"
)

// RequirementStatus represents the status of a requirement.
type RequirementStatus string

const (
	RequirementStatusNew        RequirementStatus = "new"
	RequirementStatusAnalyzed   RequirementStatus = "analyzed"
	RequirementStatusApproved   RequirementStatus = "approved"
	RequirementStatusInProgress RequirementStatus = "in_progress"
	RequirementStatusCompleted  RequirementStatus = "completed"
	RequirementStatusDeferred   RequirementStatus = "deferred"
)

// ReadDocument reads and analyzes a document.
func (a *DocumentActivities) ReadDocument(ctx context.Context, req ReadDocumentRequest) (*ReadDocumentResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Reading document", "path", req.Path)

	// Read the file
	content, err := os.ReadFile(req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Get file info
	info, err := os.Stat(req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Calculate metadata
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")
	words := strings.Fields(contentStr)

	metadata := DocumentMetadata{
		FileName:  filepath.Base(req.Path),
		FileSize:  info.Size(),
		Extension: strings.TrimPrefix(filepath.Ext(req.Path), "."),
		LineCount: len(lines),
		WordCount: len(words),
	}

	// Detect document type if not specified
	docType := req.DocumentType
	if docType == "" {
		docType = detectDocumentType(req.Path, contentStr)
	}

	// Default extract items
	extractItems := req.ExtractItems
	if len(extractItems) == 0 {
		extractItems = []string{"requirements", "features", "constraints", "dependencies"}
	}

	// Build analysis prompt
	prompt := buildDocumentAnalysisPrompt(contentStr, docType, extractItems)

	activity.RecordHeartbeat(ctx, "analyzing document with Claude")

	// Analyze with Claude
	response, err := a.claudeClient.SimpleCompletion(ctx, documentAnalysisSystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze document: %w", err)
	}

	// Parse the response
	result := &ReadDocumentResult{
		Path:          req.Path,
		Content:       contentStr,
		DocumentType:  docType,
		Metadata:      metadata,
		ExtractedData: make(map[string]interface{}),
	}

	// Try to parse structured response
	if err := parseDocumentAnalysis(response, result); err != nil {
		logger.Warn("Failed to parse structured analysis, using raw response", "error", err)
		result.Summary = response
	}

	logger.Info("Document analysis complete",
		"path", req.Path,
		"requirements_found", len(result.Requirements),
		"document_type", docType)

	return result, nil
}

// AnalyzeRequirementsRequest represents a request to analyze multiple documents for requirements.
type AnalyzeRequirementsRequest struct {
	Paths        []string `json:"paths"`
	ProjectName  string   `json:"project_name"`
	IncludeGlob  string   `json:"include_glob,omitempty"` // e.g., "*.md,*.txt,*.pdf"
	ExcludeGlob  string   `json:"exclude_glob,omitempty"`
}

// AnalyzeRequirementsResult represents the aggregated requirements analysis.
type AnalyzeRequirementsResult struct {
	ProjectName     string        `json:"project_name"`
	TotalDocuments  int           `json:"total_documents"`
	Requirements    []Requirement `json:"requirements"`
	Summary         string        `json:"summary"`
	Categories      []string      `json:"categories"`
	TechStack       []string      `json:"tech_stack,omitempty"`
	Constraints     []string      `json:"constraints,omitempty"`
	Risks           []string      `json:"risks,omitempty"`
}

// AnalyzeRequirements analyzes multiple documents to extract and consolidate requirements.
func (a *DocumentActivities) AnalyzeRequirements(ctx context.Context, req AnalyzeRequirementsRequest) (*AnalyzeRequirementsResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Analyzing requirements from multiple documents", "count", len(req.Paths))

	result := &AnalyzeRequirementsResult{
		ProjectName:    req.ProjectName,
		Requirements:   []Requirement{},
		Categories:     []string{},
		TechStack:      []string{},
		Constraints:    []string{},
		Risks:          []string{},
	}

	// Collect all document contents
	var allContent strings.Builder
	for i, path := range req.Paths {
		activity.RecordHeartbeat(ctx, fmt.Sprintf("reading document %d/%d", i+1, len(req.Paths)))

		content, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("Failed to read file", "path", path, "error", err)
			continue
		}

		allContent.WriteString(fmt.Sprintf("\n\n=== Document: %s ===\n\n", path))
		allContent.WriteString(string(content))
		result.TotalDocuments++
	}

	if result.TotalDocuments == 0 {
		return nil, fmt.Errorf("no documents could be read")
	}

	// Analyze all content together
	prompt := fmt.Sprintf(`Analyze the following project documents and extract ALL requirements for project "%s".

Documents:
%s

Provide a comprehensive analysis in JSON format:
{
  "summary": "Executive summary of the project requirements",
  "requirements": [
    {
      "id": "REQ-001",
      "title": "Short title",
      "description": "Detailed description",
      "type": "functional|non_functional|technical|business|user|constraint",
      "priority": "must|should|could|wont",
      "category": "Category name",
      "tags": ["tag1", "tag2"],
      "acceptance_criteria": ["Criterion 1", "Criterion 2"],
      "dependencies": ["REQ-XXX"]
    }
  ],
  "categories": ["List of requirement categories"],
  "tech_stack": ["Identified technologies"],
  "constraints": ["Technical and business constraints"],
  "risks": ["Identified risks and concerns"]
}`, req.ProjectName, allContent.String())

	activity.RecordHeartbeat(ctx, "analyzing requirements with Claude")

	response, err := a.claudeClient.SimpleCompletion(ctx, requirementsAnalysisSystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze requirements: %w", err)
	}

	// Parse the response
	if err := parseRequirementsAnalysis(response, result); err != nil {
		logger.Warn("Failed to parse requirements analysis", "error", err)
		result.Summary = response
	}

	logger.Info("Requirements analysis complete",
		"total_requirements", len(result.Requirements),
		"categories", len(result.Categories))

	return result, nil
}

const documentAnalysisSystemPrompt = `You are a technical document analyst. Your job is to:
1. Analyze technical documents (requirements, specifications, user stories, etc.)
2. Extract structured information including requirements, features, and constraints
3. Identify dependencies and relationships between items
4. Provide clear, actionable summaries

Always respond with well-structured JSON when possible.`

const requirementsAnalysisSystemPrompt = `You are a requirements analyst expert. Your job is to:
1. Extract and consolidate requirements from multiple documents
2. Identify functional and non-functional requirements
3. Categorize requirements appropriately
4. Assign priorities using MoSCoW method (Must, Should, Could, Won't)
5. Identify dependencies between requirements
6. Detect potential risks and constraints

Be thorough and ensure no requirement is missed. Use consistent ID formatting (REQ-XXX).`

func detectDocumentType(path, content string) DocumentType {
	lowerPath := strings.ToLower(path)
	lowerContent := strings.ToLower(content)

	switch {
	case strings.Contains(lowerPath, "requirement") || strings.Contains(lowerPath, "req"):
		return DocumentTypeRequirements
	case strings.Contains(lowerPath, "spec") || strings.Contains(lowerPath, "specification"):
		return DocumentTypeSpecification
	case strings.Contains(lowerPath, "design") || strings.Contains(lowerPath, "architecture"):
		return DocumentTypeDesign
	case strings.Contains(lowerPath, "story") || strings.Contains(lowerPath, "user"):
		return DocumentTypeUserStory
	case strings.Contains(lowerPath, "api") || strings.Contains(lowerPath, "swagger") || strings.Contains(lowerPath, "openapi"):
		return DocumentTypeAPI
	case strings.Contains(lowerContent, "as a user") || strings.Contains(lowerContent, "user story"):
		return DocumentTypeUserStory
	case strings.Contains(lowerContent, "requirement") || strings.Contains(lowerContent, "shall") || strings.Contains(lowerContent, "must"):
		return DocumentTypeRequirements
	default:
		return DocumentTypeGeneral
	}
}

func buildDocumentAnalysisPrompt(content string, docType DocumentType, extractItems []string) string {
	return fmt.Sprintf(`Analyze the following %s document and extract: %s

Document Content:
%s

Respond with JSON in this format:
{
  "summary": "Brief summary of the document",
  "document_type": "%s",
  "requirements": [
    {
      "id": "REQ-001",
      "title": "Requirement title",
      "description": "Full description",
      "type": "functional|non_functional|technical|business|user|constraint",
      "priority": "must|should|could|wont",
      "status": "new",
      "category": "Category",
      "tags": ["tag1"],
      "acceptance_criteria": ["criteria"],
      "source": "Document name or section"
    }
  ],
  "features": ["List of features mentioned"],
  "constraints": ["List of constraints"],
  "dependencies": ["External dependencies mentioned"],
  "technologies": ["Technologies mentioned"]
}`, docType, strings.Join(extractItems, ", "), content, docType)
}

func parseDocumentAnalysis(response string, result *ReadDocumentResult) error {
	// Find JSON in response
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return fmt.Errorf("no JSON found in response")
	}

	jsonStr := response[jsonStart : jsonEnd+1]

	var parsed struct {
		Summary      string        `json:"summary"`
		Requirements []Requirement `json:"requirements"`
		Features     []string      `json:"features"`
		Constraints  []string      `json:"constraints"`
		Dependencies []string      `json:"dependencies"`
		Technologies []string      `json:"technologies"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}

	result.Summary = parsed.Summary
	result.Requirements = parsed.Requirements
	result.ExtractedData["features"] = parsed.Features
	result.ExtractedData["constraints"] = parsed.Constraints
	result.ExtractedData["dependencies"] = parsed.Dependencies
	result.ExtractedData["technologies"] = parsed.Technologies

	// Set default status for requirements
	for i := range result.Requirements {
		if result.Requirements[i].Status == "" {
			result.Requirements[i].Status = RequirementStatusNew
		}
	}

	return nil
}

func parseRequirementsAnalysis(response string, result *AnalyzeRequirementsResult) error {
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return fmt.Errorf("no JSON found in response")
	}

	jsonStr := response[jsonStart : jsonEnd+1]

	var parsed struct {
		Summary      string        `json:"summary"`
		Requirements []Requirement `json:"requirements"`
		Categories   []string      `json:"categories"`
		TechStack    []string      `json:"tech_stack"`
		Constraints  []string      `json:"constraints"`
		Risks        []string      `json:"risks"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}

	result.Summary = parsed.Summary
	result.Requirements = parsed.Requirements
	result.Categories = parsed.Categories
	result.TechStack = parsed.TechStack
	result.Constraints = parsed.Constraints
	result.Risks = parsed.Risks

	return nil
}
