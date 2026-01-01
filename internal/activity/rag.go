package activity

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/memory"
	"github.com/anthropics/claude-orchestrator/internal/rag"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
)

// RAGActivities contains activities for retrieval-augmented generation.
type RAGActivities struct {
	memoryStore     memory.Store
	embeddingClient *rag.EmbeddingClient
	chunker         *rag.Chunker
}

// NewRAGActivities creates a new RAGActivities instance.
func NewRAGActivities(
	memoryStore memory.Store,
	embeddingConfig rag.EmbeddingConfig,
	chunkOpts rag.ChunkOptions,
) *RAGActivities {
	return &RAGActivities{
		memoryStore:     memoryStore,
		embeddingClient: rag.NewEmbeddingClient(embeddingConfig),
		chunker:         rag.NewChunker(chunkOpts),
	}
}

// IndexDocumentRequest represents a request to index a document.
type IndexDocumentRequest struct {
	DocumentID   string            `json:"document_id,omitempty"` // Auto-generated if empty
	Content      string            `json:"content"`
	Title        string            `json:"title,omitempty"`
	Namespace    string            `json:"namespace"` // For per-user or per-project isolation
	Metadata     map[string]string `json:"metadata,omitempty"`
	Keywords     []string          `json:"keywords,omitempty"`     // For importance calculation
	ChunkSize    int               `json:"chunk_size,omitempty"`   // Override default
	ChunkOverlap int               `json:"chunk_overlap,omitempty"` // Override default
}

// IndexDocumentResult represents the result of indexing a document.
type IndexDocumentResult struct {
	DocumentID  string    `json:"document_id"`
	ChunkCount  int       `json:"chunk_count"`
	TotalTokens int       `json:"total_tokens"`
	IndexedAt   time.Time `json:"indexed_at"`
}

// IndexDocument chunks and indexes a document for RAG.
func (a *RAGActivities) IndexDocument(ctx context.Context, req IndexDocumentRequest) (*IndexDocumentResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Indexing document", "title", req.Title, "namespace", req.Namespace)

	// Generate document ID if not provided
	docID := req.DocumentID
	if docID == "" {
		docID = uuid.New().String()
	}

	// Create chunker with optional overrides
	chunkOpts := rag.DefaultChunkOptions()
	if req.ChunkSize > 0 {
		chunkOpts.ChunkSize = req.ChunkSize
	}
	if req.ChunkOverlap > 0 {
		chunkOpts.ChunkOverlap = req.ChunkOverlap
	}
	chunker := rag.NewChunker(chunkOpts)

	// Chunk the document
	activity.RecordHeartbeat(ctx, "chunking document")
	chunks := chunker.ChunkDocument(req.Content, docID)

	if len(chunks) == 0 {
		return nil, fmt.Errorf("document produced no chunks")
	}

	// Calculate importance for each chunk
	for i := range chunks {
		chunks[i].Importance = rag.CalculateImportance(chunks[i], req.Title, req.Keywords)
		chunks[i].Metadata = req.Metadata
	}

	// Generate embeddings in batches
	activity.RecordHeartbeat(ctx, "generating embeddings")

	batchSize := 50
	totalTokens := 0

	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		// Extract texts for embedding
		texts := make([]string, len(batch))
		for j, chunk := range batch {
			texts[j] = chunk.Content
			totalTokens += rag.CountTokens(chunk.Content)
		}

		// Generate embeddings
		embeddings, err := a.embeddingClient.EmbedBatch(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embeddings: %w", err)
		}

		// Store each chunk with its embedding
		for j, chunk := range batch {
			if j >= len(embeddings) {
				continue
			}

			key := buildChunkKey(req.Namespace, docID, chunk.Index)

			storeOpts := memory.StoreOptions{
				Type:      memory.MemoryTypeSemantic,
				Namespace: req.Namespace,
				Tags: append([]string{
					"document:" + docID,
					"rag",
				}, mapToTags(req.Metadata)...),
				Embedding: embeddings[j],
				Metadata: map[string]interface{}{
					"document_id":  docID,
					"chunk_index":  chunk.Index,
					"start_char":   chunk.StartChar,
					"end_char":     chunk.EndChar,
					"importance":   chunk.Importance,
					"title":        req.Title,
				},
			}

			if err := a.memoryStore.Store(ctx, key, chunk.Content, storeOpts); err != nil {
				logger.Warn("Failed to store chunk", "key", key, "error", err)
			}
		}

		activity.RecordHeartbeat(ctx, fmt.Sprintf("indexed %d/%d chunks", end, len(chunks)))
	}

	logger.Info("Document indexed successfully",
		"document_id", docID,
		"chunk_count", len(chunks),
		"total_tokens", totalTokens)

	return &IndexDocumentResult{
		DocumentID:  docID,
		ChunkCount:  len(chunks),
		TotalTokens: totalTokens,
		IndexedAt:   time.Now(),
	}, nil
}

// SearchRequest represents a RAG search request.
type SearchRequest struct {
	Query               string   `json:"query"`
	Namespace           string   `json:"namespace"`
	TopK                int      `json:"top_k,omitempty"`        // Number of results (default 5)
	MinScore            float32  `json:"min_score,omitempty"`    // Minimum similarity score
	IncludeContext      bool     `json:"include_context"`        // Include surrounding chunks
	ContextBefore       int      `json:"context_before,omitempty"` // Chunks before match
	ContextAfter        int      `json:"context_after,omitempty"`  // Chunks after match
	FilterDocumentIDs   []string `json:"filter_document_ids,omitempty"` // Filter to specific docs
	FilterTags          []string `json:"filter_tags,omitempty"`
	WeightByImportance  bool     `json:"weight_by_importance"`   // Apply importance weighting
}

// SearchResult represents a single search result.
type SearchResult struct {
	Content     string            `json:"content"`
	DocumentID  string            `json:"document_id"`
	ChunkIndex  int               `json:"chunk_index"`
	Score       float32           `json:"score"`
	Importance  float32           `json:"importance"`
	FinalScore  float32           `json:"final_score"` // Score * Importance
	Title       string            `json:"title,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Context     []string          `json:"context,omitempty"` // Surrounding chunks if requested
}

// SearchResponse represents the complete search response.
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Query      string         `json:"query"`
	TotalFound int            `json:"total_found"`
}

// Search performs semantic search for RAG.
func (a *RAGActivities) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Searching", "query", req.Query, "namespace", req.Namespace)

	// Generate query embedding
	activity.RecordHeartbeat(ctx, "generating query embedding")
	queryEmbedding, err := a.embeddingClient.Embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Set defaults
	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	// Perform vector search
	activity.RecordHeartbeat(ctx, "searching")
	searchOpts := memory.VectorSearchOptions{
		Namespace:     req.Namespace,
		Type:          memory.MemoryTypeSemantic,
		K:             topK * 3, // Get more for filtering
		MinSimilarity: req.MinScore,
	}

	searchResults, err := a.memoryStore.VectorSearch(ctx, queryEmbedding, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// Filter and process results
	var results []SearchResult
	seenDocs := make(map[string]bool)

	for _, sr := range searchResults {
		// Extract document ID from metadata
		docID, _ := sr.Memory.Metadata["document_id"].(string)
		if docID == "" {
			continue
		}

		// Apply document filter
		if len(req.FilterDocumentIDs) > 0 {
			found := false
			for _, id := range req.FilterDocumentIDs {
				if id == docID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Apply tag filter
		if len(req.FilterTags) > 0 {
			hasAllTags := true
			for _, tag := range req.FilterTags {
				found := false
				for _, t := range sr.Memory.Tags {
					if t == tag {
						found = true
						break
					}
				}
				if !found {
					hasAllTags = false
					break
				}
			}
			if !hasAllTags {
				continue
			}
		}

		// Avoid duplicate documents if we're getting context
		docChunkKey := fmt.Sprintf("%s-%d", docID, sr.Memory.Metadata["chunk_index"])
		if seenDocs[docChunkKey] {
			continue
		}
		seenDocs[docChunkKey] = true

		chunkIndex, _ := sr.Memory.Metadata["chunk_index"].(int)
		importance, _ := sr.Memory.Metadata["importance"].(float64)
		title, _ := sr.Memory.Metadata["title"].(string)

		result := SearchResult{
			Content:    sr.Memory.Value,
			DocumentID: docID,
			ChunkIndex: chunkIndex,
			Score:      sr.Similarity,
			Importance: float32(importance),
			Title:      title,
		}

		// Calculate final score with importance weighting
		if req.WeightByImportance && importance > 0 {
			result.FinalScore = sr.Similarity * float32(importance)
		} else {
			result.FinalScore = sr.Similarity
		}

		// Get context chunks if requested
		if req.IncludeContext && (req.ContextBefore > 0 || req.ContextAfter > 0) {
			context := a.getChunkContext(ctx, req.Namespace, docID, chunkIndex, req.ContextBefore, req.ContextAfter)
			result.Context = context
		}

		results = append(results, result)
	}

	// Sort by final score
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// Limit to topK
	if len(results) > topK {
		results = results[:topK]
	}

	logger.Info("Search complete", "results", len(results))

	return &SearchResponse{
		Results:    results,
		Query:      req.Query,
		TotalFound: len(results),
	}, nil
}

// DeleteDocumentRequest represents a request to delete a document's chunks.
type DeleteDocumentRequest struct {
	DocumentID string `json:"document_id"`
	Namespace  string `json:"namespace"`
}

// DeleteDocument deletes all chunks for a document.
func (a *RAGActivities) DeleteDocument(ctx context.Context, req DeleteDocumentRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Deleting document", "document_id", req.DocumentID, "namespace", req.Namespace)

	// Query all chunks for this document
	pattern := fmt.Sprintf("rag:%s:%s:*", req.Namespace, req.DocumentID)
	keys, err := a.memoryStore.List(ctx, pattern)
	if err != nil {
		return fmt.Errorf("failed to list chunks: %w", err)
	}

	// Delete each chunk
	for _, key := range keys {
		if err := a.memoryStore.Delete(ctx, key); err != nil {
			logger.Warn("Failed to delete chunk", "key", key, "error", err)
		}
	}

	logger.Info("Document deleted", "chunks_deleted", len(keys))
	return nil
}

// GenerateRAGPromptRequest represents a request to generate a RAG prompt.
type GenerateRAGPromptRequest struct {
	Query             string        `json:"query"`
	SearchResults     []SearchResult `json:"search_results"`
	SystemContext     string        `json:"system_context,omitempty"`
	MaxContextTokens  int           `json:"max_context_tokens,omitempty"`
	IncludeSources    bool          `json:"include_sources"`
}

// GenerateRAGPromptResult represents the generated RAG prompt.
type GenerateRAGPromptResult struct {
	Prompt       string   `json:"prompt"`
	SourceDocs   []string `json:"source_docs"`
	TotalTokens  int      `json:"total_tokens"`
}

// GenerateRAGPrompt generates a prompt with retrieved context for Claude.
func (a *RAGActivities) GenerateRAGPrompt(ctx context.Context, req GenerateRAGPromptRequest) (*GenerateRAGPromptResult, error) {
	maxTokens := req.MaxContextTokens
	if maxTokens <= 0 {
		maxTokens = 4000
	}

	var contextBuilder strings.Builder
	var sourceDocs []string
	seenDocs := make(map[string]bool)
	totalTokens := 0

	// Build context from search results
	contextBuilder.WriteString("<retrieved_context>\n")

	for _, result := range req.SearchResults {
		// Check token budget
		chunkTokens := rag.CountTokens(result.Content)
		if totalTokens+chunkTokens > maxTokens {
			break
		}

		// Add document reference
		if !seenDocs[result.DocumentID] {
			sourceDocs = append(sourceDocs, result.DocumentID)
			seenDocs[result.DocumentID] = true
		}

		// Add context from surrounding chunks
		if len(result.Context) > 0 {
			for _, ctxChunk := range result.Context {
				contextBuilder.WriteString(ctxChunk)
				contextBuilder.WriteString("\n")
				totalTokens += rag.CountTokens(ctxChunk)
			}
		}

		// Add the main matched chunk
		contextBuilder.WriteString(fmt.Sprintf("<!-- Source: %s (score: %.2f) -->\n", result.DocumentID, result.Score))
		contextBuilder.WriteString(result.Content)
		contextBuilder.WriteString("\n\n")
		totalTokens += chunkTokens
	}

	contextBuilder.WriteString("</retrieved_context>\n\n")

	// Build the full prompt
	var promptBuilder strings.Builder

	if req.SystemContext != "" {
		promptBuilder.WriteString(req.SystemContext)
		promptBuilder.WriteString("\n\n")
	}

	promptBuilder.WriteString("Use the following retrieved context to answer the question. ")
	promptBuilder.WriteString("If the context doesn't contain relevant information, say so.\n\n")
	promptBuilder.WriteString(contextBuilder.String())
	promptBuilder.WriteString("Question: ")
	promptBuilder.WriteString(req.Query)
	promptBuilder.WriteString("\n\nAnswer:")

	prompt := promptBuilder.String()
	totalTokens += rag.CountTokens(prompt) - totalTokens // Add prompt tokens

	return &GenerateRAGPromptResult{
		Prompt:      prompt,
		SourceDocs:  sourceDocs,
		TotalTokens: totalTokens,
	}, nil
}

// ListDocumentsRequest represents a request to list indexed documents.
type ListDocumentsRequest struct {
	Namespace string `json:"namespace"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// DocumentInfo represents basic info about an indexed document.
type DocumentInfo struct {
	DocumentID  string    `json:"document_id"`
	Title       string    `json:"title,omitempty"`
	ChunkCount  int       `json:"chunk_count"`
	IndexedAt   time.Time `json:"indexed_at,omitempty"`
}

// ListDocumentsResult represents the list of indexed documents.
type ListDocumentsResult struct {
	Documents []DocumentInfo `json:"documents"`
	Total     int            `json:"total"`
}

// ListDocuments lists all indexed documents in a namespace.
func (a *RAGActivities) ListDocuments(ctx context.Context, req ListDocumentsRequest) (*ListDocumentsResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing documents", "namespace", req.Namespace)

	// Query for document tags
	queryOpts := memory.QueryOptions{
		Namespace: req.Namespace,
		Tags:      []string{"rag"},
		Limit:     req.Limit,
		Offset:    req.Offset,
	}

	memories, err := a.memoryStore.Query(ctx, "", queryOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}

	// Group by document ID
	docCounts := make(map[string]int)
	docTitles := make(map[string]string)
	docTimes := make(map[string]time.Time)

	for _, m := range memories {
		docID, _ := m.Metadata["document_id"].(string)
		if docID == "" {
			continue
		}
		docCounts[docID]++
		if title, ok := m.Metadata["title"].(string); ok && title != "" {
			docTitles[docID] = title
		}
		if docTimes[docID].IsZero() || m.CreatedAt.Before(docTimes[docID]) {
			docTimes[docID] = m.CreatedAt
		}
	}

	var documents []DocumentInfo
	for docID, count := range docCounts {
		documents = append(documents, DocumentInfo{
			DocumentID: docID,
			Title:      docTitles[docID],
			ChunkCount: count,
			IndexedAt:  docTimes[docID],
		})
	}

	return &ListDocumentsResult{
		Documents: documents,
		Total:     len(documents),
	}, nil
}

// Helper functions

func buildChunkKey(namespace, docID string, chunkIndex int) string {
	return fmt.Sprintf("rag:%s:%s:%d", namespace, docID, chunkIndex)
}

func mapToTags(m map[string]string) []string {
	var tags []string
	for k, v := range m {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}
	return tags
}

func (a *RAGActivities) getChunkContext(ctx context.Context, namespace, docID string, chunkIndex, before, after int) []string {
	var context []string

	// Get chunks before
	for i := chunkIndex - before; i < chunkIndex; i++ {
		if i < 0 {
			continue
		}
		key := buildChunkKey(namespace, docID, i)
		mem, err := a.memoryStore.Get(ctx, key)
		if err == nil && mem != nil {
			context = append(context, mem.Value)
		}
	}

	// Get chunks after
	for i := chunkIndex + 1; i <= chunkIndex+after; i++ {
		key := buildChunkKey(namespace, docID, i)
		mem, err := a.memoryStore.Get(ctx, key)
		if err == nil && mem != nil {
			context = append(context, mem.Value)
		}
	}

	return context
}
