// Package memory provides the memory storage interface and implementations.
package memory

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	ErrKeyNotFound    = errors.New("key not found")
	ErrStoreClosed    = errors.New("store is closed")
	ErrInvalidKey     = errors.New("invalid key")
	ErrInvalidValue   = errors.New("invalid value")
	ErrOperationFailed = errors.New("operation failed")
)

// MemoryType represents the type of memory entry.
type MemoryType string

const (
	MemoryTypeShortTerm MemoryType = "short_term"
	MemoryTypeLongTerm  MemoryType = "long_term"
	MemoryTypeWorking   MemoryType = "working"
	MemoryTypeEpisodic  MemoryType = "episodic"
	MemoryTypeSemantic  MemoryType = "semantic"
)

// Memory represents a memory entry.
type Memory struct {
	Key       string                 `json:"key"`
	Value     string                 `json:"value"`
	Type      MemoryType             `json:"type"`
	Namespace string                 `json:"namespace"`
	Tags      []string               `json:"tags"`
	Embedding []float32              `json:"embedding,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	AccessCount int                  `json:"access_count"`
}

// StoreOptions contains options for storing memory.
type StoreOptions struct {
	Type       MemoryType
	Namespace  string
	Tags       []string
	TTL        time.Duration
	Persistent bool
	Embedding  []float32
	Metadata   map[string]interface{}
}

// QueryOptions contains options for querying memory.
type QueryOptions struct {
	Namespace   string
	Type        MemoryType
	Tags        []string
	Limit       int
	Offset      int
	OrderBy     string
	OrderDesc   bool
	CreatedAfter *time.Time
	CreatedBefore *time.Time
}

// VectorSearchOptions contains options for vector similarity search.
type VectorSearchOptions struct {
	Namespace      string
	Type           MemoryType
	K              int     // Number of results
	MinSimilarity  float32 // Minimum similarity score (0-1)
	IncludeVectors bool    // Include embedding vectors in results
}

// VectorSearchResult represents a vector search result.
type VectorSearchResult struct {
	Memory     Memory  `json:"memory"`
	Similarity float32 `json:"similarity"`
}

// Store defines the interface for memory storage.
type Store interface {
	// Store stores a value with the given key.
	Store(ctx context.Context, key, value string, opts StoreOptions) error

	// Get retrieves a value by key.
	Get(ctx context.Context, key string) (*Memory, error)

	// Delete removes a value by key.
	Delete(ctx context.Context, key string) error

	// Query searches for memories matching the query options.
	Query(ctx context.Context, pattern string, opts QueryOptions) ([]Memory, error)

	// VectorSearch performs a vector similarity search.
	VectorSearch(ctx context.Context, embedding []float32, opts VectorSearchOptions) ([]VectorSearchResult, error)

	// List returns all keys matching a pattern.
	List(ctx context.Context, pattern string) ([]string, error)

	// Exists checks if a key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// Clear removes all entries in a namespace.
	Clear(ctx context.Context, namespace string) error

	// Close closes the store connection.
	Close() error

	// Health checks if the store is healthy.
	Health(ctx context.Context) error
}

// Namespace helpers for constructing memory keys.
const (
	NamespaceGlobal   = "global"
	NamespaceWorkflow = "workflow"
	NamespaceAgent    = "agent"
	NamespaceTask     = "task"
	NamespaceShared   = "shared"
)

// BuildKey constructs a namespaced key.
func BuildKey(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	key := parts[0]
	for i := 1; i < len(parts); i++ {
		key += ":" + parts[i]
	}
	return key
}

// WorkflowKey builds a workflow-scoped key.
func WorkflowKey(workflowID, key string) string {
	return BuildKey(NamespaceWorkflow, workflowID, key)
}

// AgentKey builds an agent-scoped key.
func AgentKey(workflowID, agentID, key string) string {
	return BuildKey(NamespaceWorkflow, workflowID, NamespaceAgent, agentID, key)
}

// SharedKey builds a shared key within a workflow.
func SharedKey(workflowID, key string) string {
	return BuildKey(NamespaceWorkflow, workflowID, NamespaceShared, key)
}

// GlobalKey builds a global key.
func GlobalKey(key string) string {
	return BuildKey(NamespaceGlobal, key)
}

// DefaultStoreOptions returns default store options.
func DefaultStoreOptions() StoreOptions {
	return StoreOptions{
		Type:       MemoryTypeWorking,
		Namespace:  NamespaceGlobal,
		Tags:       []string{},
		TTL:        0, // No expiration
		Persistent: false,
		Metadata:   make(map[string]interface{}),
	}
}

// DefaultQueryOptions returns default query options.
func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		Limit:     100,
		Offset:    0,
		OrderBy:   "created_at",
		OrderDesc: true,
	}
}
