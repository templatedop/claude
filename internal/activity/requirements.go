package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/memory"
	"go.temporal.io/sdk/activity"
)

// RequirementsActivities contains activities for requirements tracking.
type RequirementsActivities struct {
	store memory.Store
}

// NewRequirementsActivities creates a new RequirementsActivities instance.
func NewRequirementsActivities(store memory.Store) *RequirementsActivities {
	return &RequirementsActivities{store: store}
}

// RequirementTracking represents a tracked requirement with full history.
type RequirementTracking struct {
	Requirement    Requirement          `json:"requirement"`
	ProjectID      string               `json:"project_id"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	History        []RequirementChange  `json:"history"`
	Implementation *ImplementationInfo  `json:"implementation,omitempty"`
	TestStatus     *TestStatus          `json:"test_status,omitempty"`
}

// RequirementChange represents a change in a requirement.
type RequirementChange struct {
	Timestamp   time.Time         `json:"timestamp"`
	Field       string            `json:"field"`
	OldValue    string            `json:"old_value"`
	NewValue    string            `json:"new_value"`
	ChangedBy   string            `json:"changed_by"`
	Reason      string            `json:"reason,omitempty"`
}

// ImplementationInfo tracks implementation details for a requirement.
type ImplementationInfo struct {
	Status       string    `json:"status"` // not_started, in_progress, completed, blocked
	AssignedTo   string    `json:"assigned_to,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Files        []string  `json:"files,omitempty"`
	Commits      []string  `json:"commits,omitempty"`
	Notes        string    `json:"notes,omitempty"`
}

// TestStatus tracks test status for a requirement.
type TestStatus struct {
	HasTests     bool      `json:"has_tests"`
	TestFiles    []string  `json:"test_files,omitempty"`
	LastRun      *time.Time `json:"last_run,omitempty"`
	Passed       bool      `json:"passed"`
	Coverage     float64   `json:"coverage,omitempty"`
}

// StoreRequirementRequest represents a request to store a requirement.
type StoreRequirementRequest struct {
	ProjectID   string      `json:"project_id"`
	Requirement Requirement `json:"requirement"`
	Source      string      `json:"source,omitempty"`
}

// StoreRequirementResult represents the result of storing a requirement.
type StoreRequirementResult struct {
	RequirementID string    `json:"requirement_id"`
	IsNew         bool      `json:"is_new"`
	StoredAt      time.Time `json:"stored_at"`
}

// StoreRequirement stores or updates a requirement.
func (a *RequirementsActivities) StoreRequirement(ctx context.Context, req StoreRequirementRequest) (*StoreRequirementResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Storing requirement", "project", req.ProjectID, "id", req.Requirement.ID)

	key := requirementKey(req.ProjectID, req.Requirement.ID)
	now := time.Now()

	// Check if requirement exists
	existing, err := a.store.Get(ctx, key)
	isNew := err == memory.ErrKeyNotFound

	var tracking RequirementTracking
	if !isNew && existing != nil {
		if err := json.Unmarshal([]byte(existing.Value), &tracking); err != nil {
			logger.Warn("Failed to parse existing requirement, treating as new", "error", err)
			isNew = true
		}
	}

	if isNew {
		tracking = RequirementTracking{
			Requirement: req.Requirement,
			ProjectID:   req.ProjectID,
			CreatedAt:   now,
			UpdatedAt:   now,
			History:     []RequirementChange{},
		}
		tracking.Requirement.Source = req.Source
	} else {
		// Record changes
		changes := detectRequirementChanges(tracking.Requirement, req.Requirement)
		for _, change := range changes {
			change.Timestamp = now
			change.ChangedBy = "orchestrator"
			tracking.History = append(tracking.History, change)
		}
		tracking.Requirement = req.Requirement
		tracking.UpdatedAt = now
	}

	// Store the requirement
	data, err := json.Marshal(tracking)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal requirement: %w", err)
	}

	opts := memory.StoreOptions{
		Namespace: "requirements",
		Type:      memory.MemoryTypeLongTerm,
		Tags:      []string{req.ProjectID, string(req.Requirement.Type), string(req.Requirement.Priority)},
	}

	if err := a.store.Store(ctx, key, string(data), opts); err != nil {
		return nil, fmt.Errorf("failed to store requirement: %w", err)
	}

	// Update project index
	if err := a.updateProjectIndex(ctx, req.ProjectID, req.Requirement.ID); err != nil {
		logger.Warn("Failed to update project index", "error", err)
	}

	return &StoreRequirementResult{
		RequirementID: req.Requirement.ID,
		IsNew:         isNew,
		StoredAt:      now,
	}, nil
}

// GetRequirementRequest represents a request to get a requirement.
type GetRequirementRequest struct {
	ProjectID     string `json:"project_id"`
	RequirementID string `json:"requirement_id"`
}

// GetRequirement retrieves a requirement by ID.
func (a *RequirementsActivities) GetRequirement(ctx context.Context, req GetRequirementRequest) (*RequirementTracking, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting requirement", "project", req.ProjectID, "id", req.RequirementID)

	key := requirementKey(req.ProjectID, req.RequirementID)
	mem, err := a.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get requirement: %w", err)
	}

	var tracking RequirementTracking
	if err := json.Unmarshal([]byte(mem.Value), &tracking); err != nil {
		return nil, fmt.Errorf("failed to parse requirement: %w", err)
	}

	return &tracking, nil
}

// ListRequirementsRequest represents a request to list requirements.
type ListRequirementsRequest struct {
	ProjectID string              `json:"project_id"`
	Type      RequirementType     `json:"type,omitempty"`
	Priority  RequirementPriority `json:"priority,omitempty"`
	Status    RequirementStatus   `json:"status,omitempty"`
	Category  string              `json:"category,omitempty"`
	Tags      []string            `json:"tags,omitempty"`
}

// ListRequirementsResult represents the result of listing requirements.
type ListRequirementsResult struct {
	Requirements []RequirementTracking `json:"requirements"`
	Total        int                   `json:"total"`
	ByType       map[string]int        `json:"by_type"`
	ByPriority   map[string]int        `json:"by_priority"`
	ByStatus     map[string]int        `json:"by_status"`
}

// ListRequirements lists requirements for a project with optional filters.
func (a *RequirementsActivities) ListRequirements(ctx context.Context, req ListRequirementsRequest) (*ListRequirementsResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing requirements", "project", req.ProjectID)

	// Get project index
	indexKey := projectIndexKey(req.ProjectID)
	indexMem, err := a.store.Get(ctx, indexKey)
	if err != nil {
		if err == memory.ErrKeyNotFound {
			return &ListRequirementsResult{
				Requirements: []RequirementTracking{},
				Total:        0,
				ByType:       make(map[string]int),
				ByPriority:   make(map[string]int),
				ByStatus:     make(map[string]int),
			}, nil
		}
		return nil, fmt.Errorf("failed to get project index: %w", err)
	}

	var requirementIDs []string
	if err := json.Unmarshal([]byte(indexMem.Value), &requirementIDs); err != nil {
		return nil, fmt.Errorf("failed to parse project index: %w", err)
	}

	result := &ListRequirementsResult{
		Requirements: []RequirementTracking{},
		ByType:       make(map[string]int),
		ByPriority:   make(map[string]int),
		ByStatus:     make(map[string]int),
	}

	for _, reqID := range requirementIDs {
		key := requirementKey(req.ProjectID, reqID)
		mem, err := a.store.Get(ctx, key)
		if err != nil {
			continue
		}

		var tracking RequirementTracking
		if err := json.Unmarshal([]byte(mem.Value), &tracking); err != nil {
			continue
		}

		// Apply filters
		if req.Type != "" && tracking.Requirement.Type != req.Type {
			continue
		}
		if req.Priority != "" && tracking.Requirement.Priority != req.Priority {
			continue
		}
		if req.Status != "" && tracking.Requirement.Status != req.Status {
			continue
		}
		if req.Category != "" && tracking.Requirement.Category != req.Category {
			continue
		}
		if len(req.Tags) > 0 && !hasAnyTag(tracking.Requirement.Tags, req.Tags) {
			continue
		}

		result.Requirements = append(result.Requirements, tracking)
		result.ByType[string(tracking.Requirement.Type)]++
		result.ByPriority[string(tracking.Requirement.Priority)]++
		result.ByStatus[string(tracking.Requirement.Status)]++
	}

	result.Total = len(result.Requirements)

	// Sort by priority
	sort.Slice(result.Requirements, func(i, j int) bool {
		return priorityOrder(result.Requirements[i].Requirement.Priority) >
			priorityOrder(result.Requirements[j].Requirement.Priority)
	})

	return result, nil
}

// UpdateRequirementStatusRequest represents a request to update requirement status.
type UpdateRequirementStatusRequest struct {
	ProjectID     string            `json:"project_id"`
	RequirementID string            `json:"requirement_id"`
	Status        RequirementStatus `json:"status"`
	Reason        string            `json:"reason,omitempty"`
}

// UpdateRequirementStatus updates the status of a requirement.
func (a *RequirementsActivities) UpdateRequirementStatus(ctx context.Context, req UpdateRequirementStatusRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Updating requirement status", "project", req.ProjectID, "id", req.RequirementID, "status", req.Status)

	tracking, err := a.GetRequirement(ctx, GetRequirementRequest{
		ProjectID:     req.ProjectID,
		RequirementID: req.RequirementID,
	})
	if err != nil {
		return err
	}

	oldStatus := tracking.Requirement.Status
	tracking.Requirement.Status = req.Status
	tracking.UpdatedAt = time.Now()
	tracking.History = append(tracking.History, RequirementChange{
		Timestamp: time.Now(),
		Field:     "status",
		OldValue:  string(oldStatus),
		NewValue:  string(req.Status),
		ChangedBy: "orchestrator",
		Reason:    req.Reason,
	})

	// Store updated requirement
	data, err := json.Marshal(tracking)
	if err != nil {
		return fmt.Errorf("failed to marshal requirement: %w", err)
	}

	key := requirementKey(req.ProjectID, req.RequirementID)
	opts := memory.StoreOptions{
		Namespace: "requirements",
		Type:      memory.MemoryTypeLongTerm,
		Tags:      []string{req.ProjectID, string(tracking.Requirement.Type)},
	}

	return a.store.Store(ctx, key, string(data), opts)
}

// LinkImplementationRequest represents a request to link implementation details.
type LinkImplementationRequest struct {
	ProjectID     string             `json:"project_id"`
	RequirementID string             `json:"requirement_id"`
	Implementation ImplementationInfo `json:"implementation"`
}

// LinkImplementation links implementation details to a requirement.
func (a *RequirementsActivities) LinkImplementation(ctx context.Context, req LinkImplementationRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Linking implementation", "project", req.ProjectID, "id", req.RequirementID)

	tracking, err := a.GetRequirement(ctx, GetRequirementRequest{
		ProjectID:     req.ProjectID,
		RequirementID: req.RequirementID,
	})
	if err != nil {
		return err
	}

	tracking.Implementation = &req.Implementation
	tracking.UpdatedAt = time.Now()

	// Auto-update status based on implementation
	if req.Implementation.Status == "completed" && tracking.Requirement.Status != RequirementStatusCompleted {
		tracking.Requirement.Status = RequirementStatusCompleted
		tracking.History = append(tracking.History, RequirementChange{
			Timestamp: time.Now(),
			Field:     "status",
			OldValue:  string(RequirementStatusInProgress),
			NewValue:  string(RequirementStatusCompleted),
			ChangedBy: "orchestrator",
			Reason:    "Implementation completed",
		})
	}

	data, err := json.Marshal(tracking)
	if err != nil {
		return fmt.Errorf("failed to marshal requirement: %w", err)
	}

	key := requirementKey(req.ProjectID, req.RequirementID)
	opts := memory.StoreOptions{
		Namespace: "requirements",
		Type:      memory.MemoryTypeLongTerm,
	}

	return a.store.Store(ctx, key, string(data), opts)
}

// GetRequirementsSummaryRequest represents a request to get requirements summary.
type GetRequirementsSummaryRequest struct {
	ProjectID string `json:"project_id"`
}

// RequirementsSummary provides a summary of all requirements.
type RequirementsSummary struct {
	ProjectID        string         `json:"project_id"`
	Total            int            `json:"total"`
	ByStatus         map[string]int `json:"by_status"`
	ByPriority       map[string]int `json:"by_priority"`
	ByType           map[string]int `json:"by_type"`
	CompletionRate   float64        `json:"completion_rate"`
	MustHaveComplete int            `json:"must_have_complete"`
	MustHaveTotal    int            `json:"must_have_total"`
	RecentChanges    int            `json:"recent_changes"`
}

// GetRequirementsSummary gets a summary of requirements for a project.
func (a *RequirementsActivities) GetRequirementsSummary(ctx context.Context, req GetRequirementsSummaryRequest) (*RequirementsSummary, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting requirements summary", "project", req.ProjectID)

	list, err := a.ListRequirements(ctx, ListRequirementsRequest{ProjectID: req.ProjectID})
	if err != nil {
		return nil, err
	}

	summary := &RequirementsSummary{
		ProjectID:  req.ProjectID,
		Total:      list.Total,
		ByStatus:   list.ByStatus,
		ByPriority: list.ByPriority,
		ByType:     list.ByType,
	}

	completed := 0
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	for _, r := range list.Requirements {
		if r.Requirement.Priority == RequirementPriorityMust {
			summary.MustHaveTotal++
			if r.Requirement.Status == RequirementStatusCompleted {
				summary.MustHaveComplete++
			}
		}
		if r.Requirement.Status == RequirementStatusCompleted {
			completed++
		}
		for _, change := range r.History {
			if change.Timestamp.After(weekAgo) {
				summary.RecentChanges++
			}
		}
	}

	if summary.Total > 0 {
		summary.CompletionRate = float64(completed) / float64(summary.Total) * 100
	}

	return summary, nil
}

func (a *RequirementsActivities) updateProjectIndex(ctx context.Context, projectID, requirementID string) error {
	indexKey := projectIndexKey(projectID)

	var requirementIDs []string
	indexMem, err := a.store.Get(ctx, indexKey)
	if err == nil && indexMem != nil {
		json.Unmarshal([]byte(indexMem.Value), &requirementIDs)
	}

	// Check if already in index
	for _, id := range requirementIDs {
		if id == requirementID {
			return nil
		}
	}

	requirementIDs = append(requirementIDs, requirementID)

	data, _ := json.Marshal(requirementIDs)
	opts := memory.StoreOptions{
		Namespace: "requirements",
		Type:      memory.MemoryTypeLongTerm,
		Tags:      []string{projectID, "index"},
	}

	return a.store.Store(ctx, indexKey, string(data), opts)
}

func requirementKey(projectID, requirementID string) string {
	return fmt.Sprintf("requirements:%s:req:%s", projectID, requirementID)
}

func projectIndexKey(projectID string) string {
	return fmt.Sprintf("requirements:%s:index", projectID)
}

func detectRequirementChanges(old, new Requirement) []RequirementChange {
	var changes []RequirementChange

	if old.Title != new.Title {
		changes = append(changes, RequirementChange{Field: "title", OldValue: old.Title, NewValue: new.Title})
	}
	if old.Description != new.Description {
		changes = append(changes, RequirementChange{Field: "description", OldValue: old.Description, NewValue: new.Description})
	}
	if old.Priority != new.Priority {
		changes = append(changes, RequirementChange{Field: "priority", OldValue: string(old.Priority), NewValue: string(new.Priority)})
	}
	if old.Status != new.Status {
		changes = append(changes, RequirementChange{Field: "status", OldValue: string(old.Status), NewValue: string(new.Status)})
	}
	if old.Category != new.Category {
		changes = append(changes, RequirementChange{Field: "category", OldValue: old.Category, NewValue: new.Category})
	}

	return changes
}

func hasAnyTag(tags, queryTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}
	for _, t := range queryTags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

func priorityOrder(p RequirementPriority) int {
	switch p {
	case RequirementPriorityMust:
		return 4
	case RequirementPriorityShould:
		return 3
	case RequirementPriorityCould:
		return 2
	case RequirementPriorityWont:
		return 1
	default:
		return 0
	}
}
