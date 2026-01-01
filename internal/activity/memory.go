package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/memory"
	"go.temporal.io/sdk/activity"
)

// MemoryActivities contains activities for memory operations.
type MemoryActivities struct {
	store memory.Store
}

// NewMemoryActivities creates a new MemoryActivities instance.
func NewMemoryActivities(store memory.Store) *MemoryActivities {
	return &MemoryActivities{store: store}
}

// StoreMemoryRequest represents a request to store a memory.
type StoreMemoryRequest struct {
	Key        string                 `json:"key"`
	Value      string                 `json:"value"`
	Type       memory.MemoryType      `json:"type,omitempty"`
	Namespace  string                 `json:"namespace,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	TTLSeconds int                    `json:"ttl_seconds,omitempty"`
	Embedding  []float32              `json:"embedding,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// StoreMemoryResult represents the result of storing a memory.
type StoreMemoryResult struct {
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// Store stores a value in memory.
func (a *MemoryActivities) Store(ctx context.Context, req StoreMemoryRequest) (*StoreMemoryResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Storing memory", "key", req.Key, "namespace", req.Namespace)

	opts := memory.StoreOptions{
		Type:      req.Type,
		Namespace: req.Namespace,
		Tags:      req.Tags,
		Embedding: req.Embedding,
		Metadata:  req.Metadata,
	}

	if opts.Type == "" {
		opts.Type = memory.MemoryTypeWorking
	}
	if opts.Namespace == "" {
		opts.Namespace = memory.NamespaceGlobal
	}

	if req.TTLSeconds > 0 {
		opts.TTL = time.Duration(req.TTLSeconds) * time.Second
	}

	if err := a.store.Store(ctx, req.Key, req.Value, opts); err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	result := &StoreMemoryResult{
		Key:       memory.BuildKey(opts.Namespace, req.Key),
		Namespace: opts.Namespace,
	}

	if opts.TTL > 0 {
		expiresAt := time.Now().Add(opts.TTL)
		result.ExpiresAt = expiresAt.Format(time.RFC3339)
	}

	return result, nil
}

// GetMemoryRequest represents a request to get a memory.
type GetMemoryRequest struct {
	Key string `json:"key"`
}

// GetMemoryResult represents the result of getting a memory.
type GetMemoryResult struct {
	Found       bool                   `json:"found"`
	Value       string                 `json:"value,omitempty"`
	Type        memory.MemoryType      `json:"type,omitempty"`
	Namespace   string                 `json:"namespace,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   string                 `json:"created_at,omitempty"`
	AccessCount int                    `json:"access_count,omitempty"`
}

// Get retrieves a value from memory.
func (a *MemoryActivities) Get(ctx context.Context, req GetMemoryRequest) (*GetMemoryResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting memory", "key", req.Key)

	mem, err := a.store.Get(ctx, req.Key)
	if err != nil {
		if err == memory.ErrKeyNotFound {
			return &GetMemoryResult{Found: false}, nil
		}
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	return &GetMemoryResult{
		Found:       true,
		Value:       mem.Value,
		Type:        mem.Type,
		Namespace:   mem.Namespace,
		Tags:        mem.Tags,
		Metadata:    mem.Metadata,
		CreatedAt:   mem.CreatedAt.Format(time.RFC3339),
		AccessCount: mem.AccessCount,
	}, nil
}

// DeleteMemoryRequest represents a request to delete a memory.
type DeleteMemoryRequest struct {
	Key string `json:"key"`
}

// Delete removes a value from memory.
func (a *MemoryActivities) Delete(ctx context.Context, req DeleteMemoryRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Deleting memory", "key", req.Key)

	return a.store.Delete(ctx, req.Key)
}

// QueryMemoryRequest represents a request to query memories.
type QueryMemoryRequest struct {
	Pattern       string            `json:"pattern"`
	Namespace     string            `json:"namespace,omitempty"`
	Type          memory.MemoryType `json:"type,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Limit         int               `json:"limit,omitempty"`
	Offset        int               `json:"offset,omitempty"`
	OrderBy       string            `json:"order_by,omitempty"`
	OrderDesc     bool              `json:"order_desc,omitempty"`
	CreatedAfter  string            `json:"created_after,omitempty"`
	CreatedBefore string            `json:"created_before,omitempty"`
}

// QueryMemoryResult represents the result of querying memories.
type QueryMemoryResult struct {
	Memories []memory.Memory `json:"memories"`
	Count    int             `json:"count"`
}

// Query searches for memories matching the criteria.
func (a *MemoryActivities) Query(ctx context.Context, req QueryMemoryRequest) (*QueryMemoryResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Querying memories", "pattern", req.Pattern, "namespace", req.Namespace)

	opts := memory.QueryOptions{
		Namespace: req.Namespace,
		Type:      req.Type,
		Tags:      req.Tags,
		Limit:     req.Limit,
		Offset:    req.Offset,
		OrderBy:   req.OrderBy,
		OrderDesc: req.OrderDesc,
	}

	if req.CreatedAfter != "" {
		if t, err := time.Parse(time.RFC3339, req.CreatedAfter); err == nil {
			opts.CreatedAfter = &t
		}
	}
	if req.CreatedBefore != "" {
		if t, err := time.Parse(time.RFC3339, req.CreatedBefore); err == nil {
			opts.CreatedBefore = &t
		}
	}

	memories, err := a.store.Query(ctx, req.Pattern, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query memories: %w", err)
	}

	return &QueryMemoryResult{
		Memories: memories,
		Count:    len(memories),
	}, nil
}

// VectorSearchRequest represents a request for vector search.
type VectorSearchRequest struct {
	Embedding     []float32         `json:"embedding"`
	Namespace     string            `json:"namespace,omitempty"`
	Type          memory.MemoryType `json:"type,omitempty"`
	K             int               `json:"k,omitempty"`
	MinSimilarity float32           `json:"min_similarity,omitempty"`
}

// VectorSearchResult represents the result of vector search.
type VectorSearchResult struct {
	Results []memory.VectorSearchResult `json:"results"`
	Count   int                         `json:"count"`
}

// VectorSearch performs a vector similarity search.
func (a *MemoryActivities) VectorSearch(ctx context.Context, req VectorSearchRequest) (*VectorSearchResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Vector search", "namespace", req.Namespace, "k", req.K)

	opts := memory.VectorSearchOptions{
		Namespace:     req.Namespace,
		Type:          req.Type,
		K:             req.K,
		MinSimilarity: req.MinSimilarity,
	}

	if opts.K == 0 {
		opts.K = 10
	}

	results, err := a.store.VectorSearch(ctx, req.Embedding, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to perform vector search: %w", err)
	}

	return &VectorSearchResult{
		Results: results,
		Count:   len(results),
	}, nil
}

// ListKeysRequest represents a request to list keys.
type ListKeysRequest struct {
	Pattern string `json:"pattern"`
}

// ListKeysResult represents the result of listing keys.
type ListKeysResult struct {
	Keys  []string `json:"keys"`
	Count int      `json:"count"`
}

// ListKeys returns all keys matching a pattern.
func (a *MemoryActivities) ListKeys(ctx context.Context, req ListKeysRequest) (*ListKeysResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing keys", "pattern", req.Pattern)

	keys, err := a.store.List(ctx, req.Pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	return &ListKeysResult{
		Keys:  keys,
		Count: len(keys),
	}, nil
}

// ClearMemoryRequest represents a request to clear memory.
type ClearMemoryRequest struct {
	Namespace string `json:"namespace"`
}

// Clear removes all entries in a namespace.
func (a *MemoryActivities) Clear(ctx context.Context, req ClearMemoryRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Clearing memory", "namespace", req.Namespace)

	return a.store.Clear(ctx, req.Namespace)
}

// WorkflowMemoryHelper provides helper functions for workflow-scoped memory.
type WorkflowMemoryHelper struct {
	activities *MemoryActivities
	workflowID string
}

// NewWorkflowMemoryHelper creates a new WorkflowMemoryHelper.
func NewWorkflowMemoryHelper(activities *MemoryActivities, workflowID string) *WorkflowMemoryHelper {
	return &WorkflowMemoryHelper{
		activities: activities,
		workflowID: workflowID,
	}
}

// StoreShared stores a value in shared workflow memory.
func (h *WorkflowMemoryHelper) StoreShared(ctx context.Context, key, value string, tags []string) error {
	fullKey := memory.SharedKey(h.workflowID, key)
	_, err := h.activities.Store(ctx, StoreMemoryRequest{
		Key:       fullKey,
		Value:     value,
		Namespace: memory.NamespaceWorkflow,
		Tags:      tags,
	})
	return err
}

// GetShared retrieves a value from shared workflow memory.
func (h *WorkflowMemoryHelper) GetShared(ctx context.Context, key string) (string, bool, error) {
	fullKey := memory.SharedKey(h.workflowID, key)
	result, err := h.activities.Get(ctx, GetMemoryRequest{Key: fullKey})
	if err != nil {
		return "", false, err
	}
	return result.Value, result.Found, nil
}

// StoreAgent stores a value in agent-scoped memory.
func (h *WorkflowMemoryHelper) StoreAgent(ctx context.Context, agentID, key, value string, tags []string) error {
	fullKey := memory.AgentKey(h.workflowID, agentID, key)
	_, err := h.activities.Store(ctx, StoreMemoryRequest{
		Key:       fullKey,
		Value:     value,
		Namespace: memory.NamespaceAgent,
		Tags:      tags,
	})
	return err
}

// GetAgent retrieves a value from agent-scoped memory.
func (h *WorkflowMemoryHelper) GetAgent(ctx context.Context, agentID, key string) (string, bool, error) {
	fullKey := memory.AgentKey(h.workflowID, agentID, key)
	result, err := h.activities.Get(ctx, GetMemoryRequest{Key: fullKey})
	if err != nil {
		return "", false, err
	}
	return result.Value, result.Found, nil
}
