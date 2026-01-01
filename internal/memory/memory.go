package memory

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemoryStore is a thread-safe in-memory implementation of Store.
type InMemoryStore struct {
	mu       sync.RWMutex
	data     map[string]*Memory
	closed   bool
	cleanupInterval time.Duration
	stopCleanup chan struct{}
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	store := &InMemoryStore{
		data:            make(map[string]*Memory),
		cleanupInterval: time.Minute,
		stopCleanup:     make(chan struct{}),
	}
	go store.cleanupExpired()
	return store
}

// Store stores a value with the given key.
func (s *InMemoryStore) Store(ctx context.Context, key, value string, opts StoreOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" {
		return ErrInvalidKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	now := time.Now()
	var expiresAt *time.Time
	if opts.TTL > 0 {
		exp := now.Add(opts.TTL)
		expiresAt = &exp
	}

	fullKey := BuildKey(opts.Namespace, key)

	mem := &Memory{
		Key:       fullKey,
		Value:     value,
		Type:      opts.Type,
		Namespace: opts.Namespace,
		Tags:      opts.Tags,
		Embedding: opts.Embedding,
		Metadata:  opts.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
		AccessCount: 0,
	}

	s.data[fullKey] = mem
	return nil
}

// Get retrieves a value by key.
func (s *InMemoryStore) Get(ctx context.Context, key string) (*Memory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrStoreClosed
	}

	mem, exists := s.data[key]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrKeyNotFound
	}

	// Check expiration
	if mem.ExpiresAt != nil && time.Now().After(*mem.ExpiresAt) {
		s.Delete(ctx, key)
		return nil, ErrKeyNotFound
	}

	// Update access count
	s.mu.Lock()
	if m, ok := s.data[key]; ok {
		m.AccessCount++
	}
	s.mu.Unlock()

	return mem, nil
}

// Delete removes a value by key.
func (s *InMemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	delete(s.data, key)
	return nil
}

// Query searches for memories matching the query options.
func (s *InMemoryStore) Query(ctx context.Context, pattern string, opts QueryOptions) ([]Memory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var results []Memory
	regex, err := patternToRegex(pattern)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	for _, mem := range s.data {
		// Check expiration
		if mem.ExpiresAt != nil && now.After(*mem.ExpiresAt) {
			continue
		}

		// Match pattern
		if pattern != "" && pattern != "*" && !regex.MatchString(mem.Key) {
			continue
		}

		// Match namespace
		if opts.Namespace != "" && mem.Namespace != opts.Namespace {
			continue
		}

		// Match type
		if opts.Type != "" && mem.Type != opts.Type {
			continue
		}

		// Match tags
		if len(opts.Tags) > 0 && !hasAllTags(mem.Tags, opts.Tags) {
			continue
		}

		// Match time range
		if opts.CreatedAfter != nil && mem.CreatedAt.Before(*opts.CreatedAfter) {
			continue
		}
		if opts.CreatedBefore != nil && mem.CreatedAt.After(*opts.CreatedBefore) {
			continue
		}

		results = append(results, *mem)
	}

	// Sort results
	sortMemories(results, opts.OrderBy, opts.OrderDesc)

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) {
		return []Memory{}, nil
	}

	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, nil
}

// VectorSearch performs a vector similarity search.
func (s *InMemoryStore) VectorSearch(ctx context.Context, embedding []float32, opts VectorSearchOptions) ([]VectorSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var results []VectorSearchResult
	now := time.Now()

	for _, mem := range s.data {
		// Check expiration
		if mem.ExpiresAt != nil && now.After(*mem.ExpiresAt) {
			continue
		}

		// Skip if no embedding
		if len(mem.Embedding) == 0 {
			continue
		}

		// Match namespace and type
		if opts.Namespace != "" && mem.Namespace != opts.Namespace {
			continue
		}
		if opts.Type != "" && mem.Type != opts.Type {
			continue
		}

		// Calculate cosine similarity
		similarity := cosineSimilarity(embedding, mem.Embedding)
		if similarity < opts.MinSimilarity {
			continue
		}

		result := VectorSearchResult{
			Memory:     *mem,
			Similarity: similarity,
		}
		if !opts.IncludeVectors {
			result.Memory.Embedding = nil
		}
		results = append(results, result)
	}

	// Sort by similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Limit results
	if opts.K > 0 && opts.K < len(results) {
		results = results[:opts.K]
	}

	return results, nil
}

// List returns all keys matching a pattern.
func (s *InMemoryStore) List(ctx context.Context, pattern string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	regex, err := patternToRegex(pattern)
	if err != nil {
		return nil, err
	}

	var keys []string
	now := time.Now()
	for key, mem := range s.data {
		// Check expiration
		if mem.ExpiresAt != nil && now.After(*mem.ExpiresAt) {
			continue
		}

		if pattern == "" || pattern == "*" || regex.MatchString(key) {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)
	return keys, nil
}

// Exists checks if a key exists.
func (s *InMemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return false, ErrStoreClosed
	}

	mem, exists := s.data[key]
	if !exists {
		return false, nil
	}

	// Check expiration
	if mem.ExpiresAt != nil && time.Now().After(*mem.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

// Clear removes all entries in a namespace.
func (s *InMemoryStore) Clear(ctx context.Context, namespace string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if namespace == "" {
		s.data = make(map[string]*Memory)
		return nil
	}

	prefix := namespace + ":"
	for key := range s.data {
		if strings.HasPrefix(key, prefix) || key == namespace {
			delete(s.data, key)
		}
	}

	return nil
}

// Close closes the store.
func (s *InMemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.stopCleanup)
	s.data = nil
	return nil
}

// Health checks if the store is healthy.
func (s *InMemoryStore) Health(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// cleanupExpired periodically removes expired entries.
func (s *InMemoryStore) cleanupExpired() {
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCleanup:
			return
		case <-ticker.C:
			s.doCleanup()
		}
	}
}

func (s *InMemoryStore) doCleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	now := time.Now()
	for key, mem := range s.data {
		if mem.ExpiresAt != nil && now.After(*mem.ExpiresAt) {
			delete(s.data, key)
		}
	}
}

// Helper functions

func patternToRegex(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || pattern == "*" {
		return regexp.Compile(".*")
	}

	// Escape special regex characters except * and ?
	escaped := regexp.QuoteMeta(pattern)
	// Replace escaped \* with .* and escaped \? with .
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")

	return regexp.Compile("^" + escaped + "$")
}

func hasAllTags(memTags, queryTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range memTags {
		tagSet[t] = true
	}
	for _, t := range queryTags {
		if !tagSet[t] {
			return false
		}
	}
	return true
}

func sortMemories(memories []Memory, orderBy string, desc bool) {
	sort.Slice(memories, func(i, j int) bool {
		var less bool
		switch orderBy {
		case "key":
			less = memories[i].Key < memories[j].Key
		case "updated_at":
			less = memories[i].UpdatedAt.Before(memories[j].UpdatedAt)
		case "access_count":
			less = memories[i].AccessCount < memories[j].AccessCount
		default: // created_at
			less = memories[i].CreatedAt.Before(memories[j].CreatedAt)
		}
		if desc {
			return !less
		}
		return less
	})
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
