package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LocalConfig contains configuration for local file storage.
type LocalConfig struct {
	BasePath    string // Base directory for file storage
	BaseURL     string // Base URL for serving files (optional)
	MaxFileSize int64  // Maximum file size in bytes (0 = unlimited)
}

// LocalStore implements file storage on local filesystem.
type LocalStore struct {
	config   LocalConfig
	mu       sync.RWMutex
	metadata map[string]*FileMetadata // path -> metadata cache
}

// NewLocalStore creates a new local file store.
func NewLocalStore(config LocalConfig) (*LocalStore, error) {
	if config.BasePath == "" {
		config.BasePath = "./storage"
	}

	// Create base directory
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStore{
		config:   config,
		metadata: make(map[string]*FileMetadata),
	}, nil
}

// Upload uploads a file to local storage.
func (s *LocalStore) Upload(ctx context.Context, reader io.Reader, opts UploadOptions) (*FileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure bucket directory exists
	bucketPath := filepath.Join(s.config.BasePath, opts.Bucket)
	if err := os.MkdirAll(bucketPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bucket directory: %w", err)
	}

	// Generate file path
	filePath := opts.Path
	if filePath == "" {
		filePath = uuid.New().String()
	}
	fullPath := filepath.Join(bucketPath, filePath)

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Create file
	file, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Hash while copying
	hash := sha256.New()
	teeReader := io.TeeReader(reader, hash)

	size, err := io.Copy(file, teeReader)
	if err != nil {
		os.Remove(fullPath)
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Check max file size
	if s.config.MaxFileSize > 0 && size > s.config.MaxFileSize {
		os.Remove(fullPath)
		return nil, fmt.Errorf("file size %d exceeds maximum %d", size, s.config.MaxFileSize)
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	now := time.Now()

	meta := &FileMetadata{
		ID:          uuid.New().String(),
		Name:        filepath.Base(filePath),
		Path:        filePath,
		Bucket:      opts.Bucket,
		Size:        size,
		ContentType: opts.ContentType,
		Checksum:    checksum,
		CreatedAt:   now,
		UpdatedAt:   now,
		CustomMeta:  opts.CustomMeta,
	}

	// Cache metadata
	cacheKey := filepath.Join(opts.Bucket, filePath)
	s.metadata[cacheKey] = meta

	return meta, nil
}

// Download downloads a file from local storage.
func (s *LocalStore) Download(ctx context.Context, opts DownloadOptions) (io.ReadCloser, *FileMetadata, error) {
	fullPath := filepath.Join(s.config.BasePath, opts.Bucket, opts.Path)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrFileNotFound
		}
		return nil, nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Handle byte range
	if opts.Range != nil {
		if _, err := file.Seek(opts.Range.Start, io.SeekStart); err != nil {
			file.Close()
			return nil, nil, fmt.Errorf("failed to seek: %w", err)
		}
		// Note: caller should handle limiting reads to Range.End
	}

	meta, err := s.GetMetadata(ctx, opts.Bucket, opts.Path)
	if err != nil {
		file.Close()
		return nil, nil, err
	}

	return file, meta, nil
}

// Delete deletes a file from local storage.
func (s *LocalStore) Delete(ctx context.Context, bucket, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fullPath := filepath.Join(s.config.BasePath, bucket, path)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Remove from cache
	cacheKey := filepath.Join(bucket, path)
	delete(s.metadata, cacheKey)

	return nil
}

// GetMetadata retrieves file metadata.
func (s *LocalStore) GetMetadata(ctx context.Context, bucket, path string) (*FileMetadata, error) {
	s.mu.RLock()
	cacheKey := filepath.Join(bucket, path)
	if meta, ok := s.metadata[cacheKey]; ok {
		s.mu.RUnlock()
		return meta, nil
	}
	s.mu.RUnlock()

	fullPath := filepath.Join(s.config.BasePath, bucket, path)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Detect content type
	contentType := "application/octet-stream"
	file, err := os.Open(fullPath)
	if err == nil {
		defer file.Close()
		buffer := make([]byte, 512)
		n, _ := file.Read(buffer)
		if n > 0 {
			contentType = http.DetectContentType(buffer[:n])
		}
	}

	meta := &FileMetadata{
		ID:          path, // Use path as ID for local storage
		Name:        info.Name(),
		Path:        path,
		Bucket:      bucket,
		Size:        info.Size(),
		ContentType: contentType,
		CreatedAt:   info.ModTime(),
		UpdatedAt:   info.ModTime(),
	}

	// Cache it
	s.mu.Lock()
	s.metadata[cacheKey] = meta
	s.mu.Unlock()

	return meta, nil
}

// Exists checks if a file exists.
func (s *LocalStore) Exists(ctx context.Context, bucket, path string) (bool, error) {
	fullPath := filepath.Join(s.config.BasePath, bucket, path)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// List lists files in storage.
func (s *LocalStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	bucketPath := filepath.Join(s.config.BasePath, opts.Bucket)

	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		return nil, ErrBucketNotFound
	}

	result := &ListResult{
		Files:    []FileMetadata{},
		Prefixes: []string{},
	}

	searchPath := bucketPath
	if opts.Prefix != "" {
		searchPath = filepath.Join(bucketPath, opts.Prefix)
	}

	count := 0
	startReached := opts.StartAfter == ""

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		relPath, _ := filepath.Rel(bucketPath, path)
		if relPath == "." {
			return nil
		}

		// Handle pagination
		if !startReached {
			if relPath == opts.StartAfter {
				startReached = true
			}
			return nil
		}

		// Check max results
		if opts.MaxResults > 0 && count >= opts.MaxResults {
			result.IsTruncated = true
			return filepath.SkipAll
		}

		if info.IsDir() {
			if !opts.Recursive {
				result.Prefixes = append(result.Prefixes, relPath+"/")
				return filepath.SkipDir
			}
			return nil
		}

		meta := FileMetadata{
			ID:        relPath,
			Name:      info.Name(),
			Path:      relPath,
			Bucket:    opts.Bucket,
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
			UpdatedAt: info.ModTime(),
		}
		result.Files = append(result.Files, meta)
		result.NextCursor = relPath
		count++

		return nil
	}

	if opts.Recursive {
		if err := filepath.Walk(searchPath, walkFn); err != nil && err != filepath.SkipAll {
			return nil, fmt.Errorf("failed to list files: %w", err)
		}
	} else {
		entries, err := os.ReadDir(searchPath)
		if err != nil {
			if os.IsNotExist(err) {
				return result, nil
			}
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if opts.MaxResults > 0 && count >= opts.MaxResults {
				result.IsTruncated = true
				break
			}

			relPath := entry.Name()
			if opts.Prefix != "" {
				relPath = filepath.Join(opts.Prefix, entry.Name())
			}

			// Handle pagination
			if !startReached {
				if relPath == opts.StartAfter {
					startReached = true
				}
				continue
			}

			if entry.IsDir() {
				result.Prefixes = append(result.Prefixes, relPath+"/")
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			meta := FileMetadata{
				ID:        relPath,
				Name:      info.Name(),
				Path:      relPath,
				Bucket:    opts.Bucket,
				Size:      info.Size(),
				CreatedAt: info.ModTime(),
				UpdatedAt: info.ModTime(),
			}
			result.Files = append(result.Files, meta)
			result.NextCursor = relPath
			count++
		}
	}

	return result, nil
}

// GenerateUploadURL generates a "signed" URL for local storage.
// For local storage, this returns a file:// URL or custom base URL.
func (s *LocalStore) GenerateUploadURL(ctx context.Context, opts SignedURLOptions) (*SignedURL, error) {
	path := opts.Path
	if path == "" {
		path = uuid.New().String()
	}

	var url string
	if s.config.BaseURL != "" {
		url = fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(s.config.BaseURL, "/"), opts.Bucket, path)
	} else {
		fullPath := filepath.Join(s.config.BasePath, opts.Bucket, path)
		url = "file://" + fullPath
	}

	return &SignedURL{
		URL:       url,
		Method:    "PUT",
		ExpiresAt: time.Now().Add(opts.Expiration),
	}, nil
}

// GenerateDownloadURL generates a "signed" URL for downloading.
func (s *LocalStore) GenerateDownloadURL(ctx context.Context, opts SignedURLOptions) (*SignedURL, error) {
	// Check file exists
	exists, err := s.Exists(ctx, opts.Bucket, opts.Path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrFileNotFound
	}

	var url string
	if s.config.BaseURL != "" {
		url = fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(s.config.BaseURL, "/"), opts.Bucket, opts.Path)
	} else {
		fullPath := filepath.Join(s.config.BasePath, opts.Bucket, opts.Path)
		url = "file://" + fullPath
	}

	return &SignedURL{
		URL:       url,
		Method:    "GET",
		ExpiresAt: time.Now().Add(opts.Expiration),
	}, nil
}

// Copy copies a file within storage.
func (s *LocalStore) Copy(ctx context.Context, srcBucket, srcPath, dstBucket, dstPath string) (*FileMetadata, error) {
	srcFullPath := filepath.Join(s.config.BasePath, srcBucket, srcPath)
	dstFullPath := filepath.Join(s.config.BasePath, dstBucket, dstPath)

	// Open source
	src, err := os.Open(srcFullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dstFullPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create destination
	dst, err := os.Create(dstFullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination: %w", err)
	}
	defer dst.Close()

	// Copy with hash
	hash := sha256.New()
	tee := io.TeeReader(src, hash)
	size, err := io.Copy(dst, tee)
	if err != nil {
		return nil, fmt.Errorf("failed to copy: %w", err)
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	now := time.Now()

	return &FileMetadata{
		ID:        uuid.New().String(),
		Name:      filepath.Base(dstPath),
		Path:      dstPath,
		Bucket:    dstBucket,
		Size:      size,
		Checksum:  checksum,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// CreateBucket creates a new bucket (directory).
func (s *LocalStore) CreateBucket(ctx context.Context, bucket string) error {
	bucketPath := filepath.Join(s.config.BasePath, bucket)
	return os.MkdirAll(bucketPath, 0755)
}

// DeleteBucket deletes a bucket.
func (s *LocalStore) DeleteBucket(ctx context.Context, bucket string) error {
	bucketPath := filepath.Join(s.config.BasePath, bucket)

	// Check if empty
	entries, err := os.ReadDir(bucketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrBucketNotFound
		}
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("bucket is not empty")
	}

	return os.Remove(bucketPath)
}

// ListBuckets lists all buckets.
func (s *LocalStore) ListBuckets(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.config.BasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	var buckets []string
	for _, entry := range entries {
		if entry.IsDir() {
			buckets = append(buckets, entry.Name())
		}
	}

	return buckets, nil
}

// Health checks if the store is healthy.
func (s *LocalStore) Health(ctx context.Context) error {
	_, err := os.Stat(s.config.BasePath)
	return err
}

// Close closes the store.
func (s *LocalStore) Close() error {
	return nil
}

// Type returns the storage type.
func (s *LocalStore) Type() StorageType {
	return StorageTypeLocal
}
