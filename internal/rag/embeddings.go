package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingProvider represents the embedding model provider.
type EmbeddingProvider string

const (
	EmbeddingProviderOpenAI    EmbeddingProvider = "openai"
	EmbeddingProviderClaude    EmbeddingProvider = "claude" // Claude doesn't have embeddings, use fallback
	EmbeddingProviderCohere    EmbeddingProvider = "cohere"
	EmbeddingProviderLocal     EmbeddingProvider = "local"   // Local model
	EmbeddingProviderSentence  EmbeddingProvider = "sentence" // Sentence transformers
)

// EmbeddingModel represents specific embedding models.
type EmbeddingModel string

const (
	// OpenAI models
	ModelTextEmbedding3Small EmbeddingModel = "text-embedding-3-small" // 1536 dims
	ModelTextEmbedding3Large EmbeddingModel = "text-embedding-3-large" // 3072 dims
	ModelTextEmbeddingAda002 EmbeddingModel = "text-embedding-ada-002" // 1536 dims

	// Cohere models
	ModelCohereEmbedEnglish EmbeddingModel = "embed-english-v3.0"  // 1024 dims
	ModelCohereEmbedMulti   EmbeddingModel = "embed-multilingual-v3.0" // 1024 dims
)

// EmbeddingDimensions maps models to their embedding dimensions.
var EmbeddingDimensions = map[EmbeddingModel]int{
	ModelTextEmbedding3Small: 1536,
	ModelTextEmbedding3Large: 3072,
	ModelTextEmbeddingAda002: 1536,
	ModelCohereEmbedEnglish:  1024,
	ModelCohereEmbedMulti:    1024,
}

// EmbeddingConfig contains configuration for embedding generation.
type EmbeddingConfig struct {
	Provider   EmbeddingProvider
	Model      EmbeddingModel
	APIKey     string
	Endpoint   string // Custom endpoint for local/self-hosted
	Dimensions int    // Override default dimensions
	BatchSize  int    // Max texts per request
	Timeout    time.Duration
}

// EmbeddingClient generates embeddings for text.
type EmbeddingClient struct {
	config     EmbeddingConfig
	httpClient *http.Client
}

// NewEmbeddingClient creates a new embedding client.
func NewEmbeddingClient(config EmbeddingConfig) *EmbeddingClient {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.Dimensions == 0 {
		if dim, ok := EmbeddingDimensions[config.Model]; ok {
			config.Dimensions = dim
		} else {
			config.Dimensions = 1536 // Default
		}
	}

	return &EmbeddingClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Embed generates embeddings for a single text.
func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (c *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	switch c.config.Provider {
	case EmbeddingProviderOpenAI:
		return c.embedOpenAI(ctx, texts)
	case EmbeddingProviderCohere:
		return c.embedCohere(ctx, texts)
	case EmbeddingProviderLocal:
		return c.embedLocal(ctx, texts)
	default:
		return c.embedOpenAI(ctx, texts) // Default to OpenAI
	}
}

// Dimensions returns the embedding dimensions for the configured model.
func (c *EmbeddingClient) Dimensions() int {
	return c.config.Dimensions
}

// embedOpenAI generates embeddings using OpenAI API.
func (c *EmbeddingClient) embedOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	endpoint := c.config.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/embeddings"
	}

	model := string(c.config.Model)
	if model == "" {
		model = string(ModelTextEmbedding3Small)
	}

	requestBody := map[string]interface{}{
		"input": texts,
		"model": model,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Sort by index and extract embeddings
	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}

	return embeddings, nil
}

// embedCohere generates embeddings using Cohere API.
func (c *EmbeddingClient) embedCohere(ctx context.Context, texts []string) ([][]float32, error) {
	endpoint := c.config.Endpoint
	if endpoint == "" {
		endpoint = "https://api.cohere.ai/v1/embed"
	}

	model := string(c.config.Model)
	if model == "" {
		model = string(ModelCohereEmbedEnglish)
	}

	requestBody := map[string]interface{}{
		"texts":      texts,
		"model":      model,
		"input_type": "search_document",
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Embeddings, nil
}

// embedLocal generates embeddings using a local endpoint.
func (c *EmbeddingClient) embedLocal(ctx context.Context, texts []string) ([][]float32, error) {
	if c.config.Endpoint == "" {
		return nil, fmt.Errorf("local endpoint not configured")
	}

	requestBody := map[string]interface{}{
		"texts": texts,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Embeddings, nil
}

// CosineSimilarity calculates cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt32(normA) * sqrt32(normB))
}

func sqrt32(x float32) float32 {
	// Simple Newton-Raphson for float32
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// NormalizeEmbedding normalizes an embedding vector to unit length.
func NormalizeEmbedding(embedding []float32) []float32 {
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	norm = sqrt32(norm)

	if norm == 0 {
		return embedding
	}

	normalized := make([]float32, len(embedding))
	for i, v := range embedding {
		normalized[i] = v / norm
	}
	return normalized
}
