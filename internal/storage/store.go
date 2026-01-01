// Package storage provides cloud file storage abstraction.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Common errors.
var (
	ErrFileNotFound     = errors.New("file not found")
	ErrBucketNotFound   = errors.New("bucket not found")
	ErrAccessDenied     = errors.New("access denied")
	ErrInvalidPath      = errors.New("invalid path")
	ErrUploadFailed     = errors.New("upload failed")
	ErrDownloadFailed   = errors.New("download failed")
	ErrStoreClosed      = errors.New("store is closed")
)

// StorageType represents the type of storage backend.
type StorageType string

const (
	StorageTypeLocal  StorageType = "local"
	StorageTypeS3     StorageType = "s3"
	StorageTypeGCS    StorageType = "gcs"
	StorageTypeAzure  StorageType = "azure"
	StorageTypeMinIO  StorageType = "minio"
)

// FileMetadata contains metadata about a stored file.
type FileMetadata struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Bucket       string            `json:"bucket"`
	Size         int64             `json:"size"`
	ContentType  string            `json:"content_type"`
	Checksum     string            `json:"checksum"`
	ETag         string            `json:"etag,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	CustomMeta   map[string]string `json:"custom_metadata,omitempty"`
	StorageClass string            `json:"storage_class,omitempty"`
}

// UploadOptions contains options for file upload.
type UploadOptions struct {
	ContentType  string
	Bucket       string
	Path         string
	CustomMeta   map[string]string
	StorageClass string
	Public       bool   // Whether the file should be publicly accessible
	TTL          time.Duration // Auto-delete after TTL (0 = no expiration)
}

// DownloadOptions contains options for file download.
type DownloadOptions struct {
	Bucket string
	Path   string
	Range  *ByteRange // For partial downloads
}

// ByteRange represents a byte range for partial downloads.
type ByteRange struct {
	Start int64
	End   int64
}

// SignedURLOptions contains options for generating signed URLs.
type SignedURLOptions struct {
	Bucket      string
	Path        string
	Expiration  time.Duration
	ContentType string            // Required content type for upload URLs
	Method      string            // GET for download, PUT for upload
	Headers     map[string]string // Custom headers to include
}

// SignedURL represents a signed URL for file access.
type SignedURL struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	ExpiresAt time.Time         `json:"expires_at"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// ListOptions contains options for listing files.
type ListOptions struct {
	Bucket      string
	Prefix      string
	Delimiter   string
	MaxResults  int
	StartAfter  string // Pagination cursor
	Recursive   bool
}

// ListResult contains the result of listing files.
type ListResult struct {
	Files       []FileMetadata `json:"files"`
	Prefixes    []string       `json:"prefixes,omitempty"` // Common prefixes (directories)
	NextCursor  string         `json:"next_cursor,omitempty"`
	IsTruncated bool           `json:"is_truncated"`
}

// Store defines the interface for cloud file storage.
type Store interface {
	// Upload uploads a file to storage.
	Upload(ctx context.Context, reader io.Reader, opts UploadOptions) (*FileMetadata, error)

	// Download downloads a file from storage.
	Download(ctx context.Context, opts DownloadOptions) (io.ReadCloser, *FileMetadata, error)

	// Delete deletes a file from storage.
	Delete(ctx context.Context, bucket, path string) error

	// GetMetadata retrieves file metadata without downloading.
	GetMetadata(ctx context.Context, bucket, path string) (*FileMetadata, error)

	// Exists checks if a file exists.
	Exists(ctx context.Context, bucket, path string) (bool, error)

	// List lists files in storage.
	List(ctx context.Context, opts ListOptions) (*ListResult, error)

	// GenerateUploadURL generates a signed URL for uploading.
	GenerateUploadURL(ctx context.Context, opts SignedURLOptions) (*SignedURL, error)

	// GenerateDownloadURL generates a signed URL for downloading.
	GenerateDownloadURL(ctx context.Context, opts SignedURLOptions) (*SignedURL, error)

	// Copy copies a file within storage.
	Copy(ctx context.Context, srcBucket, srcPath, dstBucket, dstPath string) (*FileMetadata, error)

	// CreateBucket creates a new bucket.
	CreateBucket(ctx context.Context, bucket string) error

	// DeleteBucket deletes a bucket.
	DeleteBucket(ctx context.Context, bucket string) error

	// ListBuckets lists all buckets.
	ListBuckets(ctx context.Context) ([]string, error)

	// Health checks if the store is healthy.
	Health(ctx context.Context) error

	// Close closes the store connection.
	Close() error

	// Type returns the storage type.
	Type() StorageType
}

// DefaultUploadOptions returns default upload options.
func DefaultUploadOptions() UploadOptions {
	return UploadOptions{
		ContentType:  "application/octet-stream",
		Bucket:       "default",
		CustomMeta:   make(map[string]string),
		StorageClass: "STANDARD",
		Public:       false,
	}
}

// DefaultListOptions returns default list options.
func DefaultListOptions() ListOptions {
	return ListOptions{
		Bucket:     "default",
		MaxResults: 1000,
		Recursive:  false,
	}
}

// DefaultSignedURLOptions returns default signed URL options.
func DefaultSignedURLOptions() SignedURLOptions {
	return SignedURLOptions{
		Bucket:     "default",
		Expiration: 1 * time.Hour,
		Method:     "GET",
	}
}
