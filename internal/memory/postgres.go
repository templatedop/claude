package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	db     *sql.DB
	schema string
}

// PostgresConfig holds PostgreSQL connection configuration.
type PostgresConfig struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	SSLMode         string
	Schema          string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultPostgresConfig returns default PostgreSQL configuration.
func DefaultPostgresConfig() PostgresConfig {
	return PostgresConfig{
		Host:            "localhost",
		Port:            5432,
		Database:        "claude_orchestrator",
		User:            "postgres",
		Password:        "",
		SSLMode:         "disable",
		Schema:          "memory",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	}
}

// NewPostgresStore creates a new PostgreSQL store.
func NewPostgresStore(cfg PostgresConfig) (*PostgresStore, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &PostgresStore{
		db:     db,
		schema: cfg.Schema,
	}

	if err := store.initSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the necessary tables and indexes.
func (s *PostgresStore) initSchema(ctx context.Context) error {
	queries := []string{
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, s.schema),
		fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.memories (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				type TEXT NOT NULL DEFAULT 'working',
				namespace TEXT NOT NULL DEFAULT 'global',
				tags TEXT[] DEFAULT '{}',
				embedding vector(1536),
				metadata JSONB DEFAULT '{}',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMPTZ,
				access_count INTEGER NOT NULL DEFAULT 0
			)
		`, s.schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_memories_namespace ON %s.memories(namespace)`, s.schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_memories_type ON %s.memories(type)`, s.schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_memories_tags ON %s.memories USING GIN(tags)`, s.schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_memories_created_at ON %s.memories(created_at)`, s.schema),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_memories_expires_at ON %s.memories(expires_at) WHERE expires_at IS NOT NULL`, s.schema),
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			// Ignore "extension does not exist" errors for vector
			if !strings.Contains(err.Error(), "vector") {
				return fmt.Errorf("failed to execute query: %w", err)
			}
		}
	}

	return nil
}

// Store stores a value with the given key.
func (s *PostgresStore) Store(ctx context.Context, key, value string, opts StoreOptions) error {
	if key == "" {
		return ErrInvalidKey
	}

	fullKey := BuildKey(opts.Namespace, key)

	var expiresAt *time.Time
	if opts.TTL > 0 {
		exp := time.Now().Add(opts.TTL)
		expiresAt = &exp
	}

	metadataJSON, err := json.Marshal(opts.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s.memories (key, value, type, namespace, tags, metadata, expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			type = EXCLUDED.type,
			namespace = EXCLUDED.namespace,
			tags = EXCLUDED.tags,
			metadata = EXCLUDED.metadata,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW(),
			access_count = %s.memories.access_count
	`, s.schema, s.schema)

	memType := string(opts.Type)
	if memType == "" {
		memType = string(MemoryTypeWorking)
	}

	_, err = s.db.ExecContext(ctx, query,
		fullKey,
		value,
		memType,
		opts.Namespace,
		pq.Array(opts.Tags),
		metadataJSON,
		expiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	return nil
}

// Get retrieves a value by key.
func (s *PostgresStore) Get(ctx context.Context, key string) (*Memory, error) {
	query := fmt.Sprintf(`
		UPDATE %s.memories
		SET access_count = access_count + 1
		WHERE key = $1 AND (expires_at IS NULL OR expires_at > NOW())
		RETURNING key, value, type, namespace, tags, metadata, created_at, updated_at, expires_at, access_count
	`, s.schema)

	var mem Memory
	var tags pq.StringArray
	var metadataJSON []byte
	var expiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, query, key).Scan(
		&mem.Key,
		&mem.Value,
		&mem.Type,
		&mem.Namespace,
		&tags,
		&metadataJSON,
		&mem.CreatedAt,
		&mem.UpdatedAt,
		&expiresAt,
		&mem.AccessCount,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	mem.Tags = tags
	if expiresAt.Valid {
		mem.ExpiresAt = &expiresAt.Time
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &mem.Metadata); err != nil {
			mem.Metadata = make(map[string]interface{})
		}
	}

	return &mem, nil
}

// Delete removes a value by key.
func (s *PostgresStore) Delete(ctx context.Context, key string) error {
	query := fmt.Sprintf(`DELETE FROM %s.memories WHERE key = $1`, s.schema)
	_, err := s.db.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}
	return nil
}

// Query searches for memories matching the query options.
func (s *PostgresStore) Query(ctx context.Context, pattern string, opts QueryOptions) ([]Memory, error) {
	var conditions []string
	var args []interface{}
	argNum := 1

	// Expiration check
	conditions = append(conditions, "(expires_at IS NULL OR expires_at > NOW())")

	// Pattern matching
	if pattern != "" && pattern != "*" {
		// Convert glob pattern to SQL LIKE pattern
		likePattern := strings.ReplaceAll(pattern, "*", "%")
		likePattern = strings.ReplaceAll(likePattern, "?", "_")
		conditions = append(conditions, fmt.Sprintf("key LIKE $%d", argNum))
		args = append(args, likePattern)
		argNum++
	}

	// Namespace filter
	if opts.Namespace != "" {
		conditions = append(conditions, fmt.Sprintf("namespace = $%d", argNum))
		args = append(args, opts.Namespace)
		argNum++
	}

	// Type filter
	if opts.Type != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argNum))
		args = append(args, string(opts.Type))
		argNum++
	}

	// Tags filter
	if len(opts.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("tags @> $%d", argNum))
		args = append(args, pq.Array(opts.Tags))
		argNum++
	}

	// Time range filters
	if opts.CreatedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("created_at > $%d", argNum))
		args = append(args, opts.CreatedAfter)
		argNum++
	}
	if opts.CreatedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argNum))
		args = append(args, opts.CreatedBefore)
		argNum++
	}

	// Build query
	whereClause := strings.Join(conditions, " AND ")

	orderBy := "created_at"
	if opts.OrderBy != "" {
		orderBy = opts.OrderBy
	}
	orderDir := "ASC"
	if opts.OrderDesc {
		orderDir = "DESC"
	}

	limit := 100
	if opts.Limit > 0 {
		limit = opts.Limit
	}

	query := fmt.Sprintf(`
		SELECT key, value, type, namespace, tags, metadata, created_at, updated_at, expires_at, access_count
		FROM %s.memories
		WHERE %s
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, s.schema, whereClause, orderBy, orderDir, limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var mem Memory
		var tags pq.StringArray
		var metadataJSON []byte
		var expiresAt sql.NullTime

		err := rows.Scan(
			&mem.Key,
			&mem.Value,
			&mem.Type,
			&mem.Namespace,
			&tags,
			&metadataJSON,
			&mem.CreatedAt,
			&mem.UpdatedAt,
			&expiresAt,
			&mem.AccessCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory: %w", err)
		}

		mem.Tags = tags
		if expiresAt.Valid {
			mem.ExpiresAt = &expiresAt.Time
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &mem.Metadata); err != nil {
				mem.Metadata = make(map[string]interface{})
			}
		}

		memories = append(memories, mem)
	}

	return memories, rows.Err()
}

// VectorSearch performs a vector similarity search using pgvector.
func (s *PostgresStore) VectorSearch(ctx context.Context, embedding []float32, opts VectorSearchOptions) ([]VectorSearchResult, error) {
	if len(embedding) == 0 {
		return nil, errors.New("embedding is empty")
	}

	var conditions []string
	var args []interface{}
	argNum := 1

	// Expiration check
	conditions = append(conditions, "(expires_at IS NULL OR expires_at > NOW())")

	// Embedding must exist
	conditions = append(conditions, "embedding IS NOT NULL")

	// Namespace filter
	if opts.Namespace != "" {
		conditions = append(conditions, fmt.Sprintf("namespace = $%d", argNum))
		args = append(args, opts.Namespace)
		argNum++
	}

	// Type filter
	if opts.Type != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argNum))
		args = append(args, string(opts.Type))
		argNum++
	}

	// Convert embedding to vector string
	embeddingStr := fmt.Sprintf("[%s]", floatsToString(embedding))
	args = append(args, embeddingStr)

	whereClause := strings.Join(conditions, " AND ")

	k := 10
	if opts.K > 0 {
		k = opts.K
	}

	// Use cosine distance (1 - similarity)
	query := fmt.Sprintf(`
		SELECT key, value, type, namespace, tags, metadata, created_at, updated_at, expires_at, access_count,
			   1 - (embedding <=> $%d::vector) as similarity
		FROM %s.memories
		WHERE %s
		ORDER BY embedding <=> $%d::vector
		LIMIT %d
	`, argNum, s.schema, whereClause, argNum, k)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		// If vector extension is not available, return empty results
		if strings.Contains(err.Error(), "vector") {
			return []VectorSearchResult{}, nil
		}
		return nil, fmt.Errorf("failed to perform vector search: %w", err)
	}
	defer rows.Close()

	var results []VectorSearchResult
	for rows.Next() {
		var mem Memory
		var tags pq.StringArray
		var metadataJSON []byte
		var expiresAt sql.NullTime
		var similarity float32

		err := rows.Scan(
			&mem.Key,
			&mem.Value,
			&mem.Type,
			&mem.Namespace,
			&tags,
			&metadataJSON,
			&mem.CreatedAt,
			&mem.UpdatedAt,
			&expiresAt,
			&mem.AccessCount,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}

		// Apply minimum similarity filter
		if similarity < opts.MinSimilarity {
			continue
		}

		mem.Tags = tags
		if expiresAt.Valid {
			mem.ExpiresAt = &expiresAt.Time
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &mem.Metadata); err != nil {
				mem.Metadata = make(map[string]interface{})
			}
		}

		results = append(results, VectorSearchResult{
			Memory:     mem,
			Similarity: similarity,
		})
	}

	return results, rows.Err()
}

// List returns all keys matching a pattern.
func (s *PostgresStore) List(ctx context.Context, pattern string) ([]string, error) {
	var query string
	var args []interface{}

	if pattern == "" || pattern == "*" {
		query = fmt.Sprintf(`
			SELECT key FROM %s.memories
			WHERE expires_at IS NULL OR expires_at > NOW()
			ORDER BY key
		`, s.schema)
	} else {
		likePattern := strings.ReplaceAll(pattern, "*", "%")
		likePattern = strings.ReplaceAll(likePattern, "?", "_")
		query = fmt.Sprintf(`
			SELECT key FROM %s.memories
			WHERE (expires_at IS NULL OR expires_at > NOW()) AND key LIKE $1
			ORDER BY key
		`, s.schema)
		args = append(args, likePattern)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

// Exists checks if a key exists.
func (s *PostgresStore) Exists(ctx context.Context, key string) (bool, error) {
	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM %s.memories
			WHERE key = $1 AND (expires_at IS NULL OR expires_at > NOW())
		)
	`, s.schema)

	var exists bool
	err := s.db.QueryRowContext(ctx, query, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return exists, nil
}

// Clear removes all entries in a namespace.
func (s *PostgresStore) Clear(ctx context.Context, namespace string) error {
	var query string
	var args []interface{}

	if namespace == "" {
		query = fmt.Sprintf(`TRUNCATE %s.memories`, s.schema)
	} else {
		query = fmt.Sprintf(`DELETE FROM %s.memories WHERE namespace = $1 OR key LIKE $2`, s.schema)
		args = append(args, namespace, namespace+":%")
	}

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to clear memories: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// Health checks if the database is healthy.
func (s *PostgresStore) Health(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// CleanupExpired removes all expired entries.
func (s *PostgresStore) CleanupExpired(ctx context.Context) (int64, error) {
	query := fmt.Sprintf(`DELETE FROM %s.memories WHERE expires_at IS NOT NULL AND expires_at <= NOW()`, s.schema)
	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired memories: %w", err)
	}
	return result.RowsAffected()
}

func floatsToString(floats []float32) string {
	strs := make([]string, len(floats))
	for i, f := range floats {
		strs[i] = fmt.Sprintf("%f", f)
	}
	return strings.Join(strs, ",")
}
