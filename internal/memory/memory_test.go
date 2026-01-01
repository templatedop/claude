package memory

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStore_Store(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Test basic store
	err := store.Store(ctx, "test-key", "test-value", DefaultStoreOptions())
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Test store with options
	opts := StoreOptions{
		Type:      MemoryTypeLongTerm,
		Namespace: "test-ns",
		Tags:      []string{"tag1", "tag2"},
		TTL:       time.Hour,
		Metadata:  map[string]interface{}{"key": "value"},
	}
	err = store.Store(ctx, "test-key-2", "test-value-2", opts)
	if err != nil {
		t.Fatalf("Store with options failed: %v", err)
	}
}

func TestInMemoryStore_Get(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store a value
	opts := StoreOptions{
		Namespace: "test",
		Type:      MemoryTypeWorking,
	}
	err := store.Store(ctx, "get-test", "hello world", opts)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Get the value
	key := BuildKey("test", "get-test")
	mem, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if mem.Value != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", mem.Value)
	}

	if mem.Type != MemoryTypeWorking {
		t.Errorf("Expected type 'working', got '%s'", mem.Type)
	}

	// Test get non-existent key
	_, err = store.Get(ctx, "non-existent")
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound, got %v", err)
	}
}

func TestInMemoryStore_Delete(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store and delete
	opts := DefaultStoreOptions()
	err := store.Store(ctx, "delete-test", "value", opts)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	key := BuildKey(opts.Namespace, "delete-test")
	err = store.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.Get(ctx, key)
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound after delete, got %v", err)
	}
}

func TestInMemoryStore_Query(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store multiple values
	opts := StoreOptions{
		Namespace: "query-test",
		Type:      MemoryTypeWorking,
		Tags:      []string{"important"},
	}

	for i := 0; i < 5; i++ {
		err := store.Store(ctx, "key-"+string(rune('a'+i)), "value", opts)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	// Query all
	results, err := store.Query(ctx, "*", QueryOptions{
		Namespace: "query-test",
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	// Query with limit
	results, err = store.Query(ctx, "*", QueryOptions{
		Namespace: "query-test",
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("Query with limit failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results with limit, got %d", len(results))
	}

	// Query with tags
	results, err = store.Query(ctx, "*", QueryOptions{
		Tags: []string{"important"},
	})
	if err != nil {
		t.Fatalf("Query with tags failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results with tag filter, got %d", len(results))
	}
}

func TestInMemoryStore_VectorSearch(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store values with embeddings
	vectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.9, 0.1, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
	}

	for i, vec := range vectors {
		opts := StoreOptions{
			Namespace: "vector-test",
			Embedding: vec,
		}
		err := store.Store(ctx, "vec-"+string(rune('a'+i)), "value", opts)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	// Search for similar vectors
	query := []float32{1.0, 0.0, 0.0}
	results, err := store.VectorSearch(ctx, query, VectorSearchOptions{
		K:             2,
		MinSimilarity: 0.5,
	})
	if err != nil {
		t.Fatalf("VectorSearch failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// First result should be exact match
	if results[0].Similarity < 0.99 {
		t.Errorf("Expected similarity ~1.0, got %f", results[0].Similarity)
	}
}

func TestInMemoryStore_TTL(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store with very short TTL
	opts := StoreOptions{
		Namespace: "ttl-test",
		TTL:       10 * time.Millisecond,
	}
	err := store.Store(ctx, "expires-soon", "value", opts)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Should exist immediately
	key := BuildKey("ttl-test", "expires-soon")
	_, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed before expiry: %v", err)
	}

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	// Should not exist after expiry
	_, err = store.Get(ctx, key)
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound after TTL expiry, got %v", err)
	}
}

func TestInMemoryStore_List(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store values
	opts := StoreOptions{Namespace: "list-test"}
	store.Store(ctx, "key1", "v1", opts)
	store.Store(ctx, "key2", "v2", opts)
	store.Store(ctx, "other", "v3", opts)

	// List all
	keys, err := store.List(ctx, "*")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// List with pattern
	keys, err = store.List(ctx, "*key*")
	if err != nil {
		t.Fatalf("List with pattern failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys matching pattern, got %d", len(keys))
	}
}

func TestInMemoryStore_Exists(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	opts := StoreOptions{Namespace: "exists-test"}
	store.Store(ctx, "exists", "value", opts)

	key := BuildKey("exists-test", "exists")
	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if !exists {
		t.Error("Expected key to exist")
	}

	exists, err = store.Exists(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Exists for non-existent failed: %v", err)
	}

	if exists {
		t.Error("Expected key to not exist")
	}
}

func TestInMemoryStore_Clear(t *testing.T) {
	store := NewInMemoryStore()
	defer store.Close()

	ctx := context.Background()

	// Store in different namespaces
	store.Store(ctx, "key1", "v1", StoreOptions{Namespace: "ns1"})
	store.Store(ctx, "key2", "v2", StoreOptions{Namespace: "ns1"})
	store.Store(ctx, "key3", "v3", StoreOptions{Namespace: "ns2"})

	// Clear one namespace
	err := store.Clear(ctx, "ns1")
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// ns1 keys should be gone
	_, err = store.Get(ctx, BuildKey("ns1", "key1"))
	if err != ErrKeyNotFound {
		t.Error("Expected ns1 key to be cleared")
	}

	// ns2 key should still exist
	_, err = store.Get(ctx, BuildKey("ns2", "key3"))
	if err != nil {
		t.Error("Expected ns2 key to still exist")
	}
}

func TestBuildKey(t *testing.T) {
	tests := []struct {
		parts    []string
		expected string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a:b"},
		{[]string{"a", "b", "c"}, "a:b:c"},
		{[]string{}, ""},
	}

	for _, tt := range tests {
		result := BuildKey(tt.parts...)
		if result != tt.expected {
			t.Errorf("BuildKey(%v) = %s, want %s", tt.parts, result, tt.expected)
		}
	}
}

func TestWorkflowKey(t *testing.T) {
	key := WorkflowKey("wf-123", "state")
	expected := "workflow:wf-123:state"
	if key != expected {
		t.Errorf("WorkflowKey = %s, want %s", key, expected)
	}
}

func TestAgentKey(t *testing.T) {
	key := AgentKey("wf-123", "agent-1", "memory")
	expected := "workflow:wf-123:agent:agent-1:memory"
	if key != expected {
		t.Errorf("AgentKey = %s, want %s", key, expected)
	}
}

func TestSharedKey(t *testing.T) {
	key := SharedKey("wf-123", "context")
	expected := "workflow:wf-123:shared:context"
	if key != expected {
		t.Errorf("SharedKey = %s, want %s", key, expected)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		a, b     []float32
		expected float32
	}{
		{[]float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{[]float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{[]float32{1, 0, 0}, []float32{-1, 0, 0}, -1.0},
		{[]float32{1, 1, 0}, []float32{1, 0, 0}, 0.7071068}, // sqrt(2)/2
	}

	for _, tt := range tests {
		result := cosineSimilarity(tt.a, tt.b)
		diff := result - tt.expected
		if diff < -0.001 || diff > 0.001 {
			t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, result, tt.expected)
		}
	}
}
