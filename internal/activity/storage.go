package activity

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/storage"
	"go.temporal.io/sdk/activity"
)

// StorageActivities contains activities for cloud file storage operations.
type StorageActivities struct {
	store storage.Store
}

// NewStorageActivities creates a new StorageActivities instance.
func NewStorageActivities(store storage.Store) *StorageActivities {
	return &StorageActivities{store: store}
}

// UploadFileRequest represents a request to upload a file.
type UploadFileRequest struct {
	Content     []byte            `json:"content"`
	Bucket      string            `json:"bucket"`
	Path        string            `json:"path,omitempty"` // Auto-generated if empty
	ContentType string            `json:"content_type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Public      bool              `json:"public,omitempty"`
	TTLSeconds  int               `json:"ttl_seconds,omitempty"`
}

// UploadFileResult represents the result of uploading a file.
type UploadFileResult struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	Bucket      string    `json:"bucket"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	Checksum    string    `json:"checksum"`
	CreatedAt   time.Time `json:"created_at"`
}

// Upload uploads a file to cloud storage.
func (a *StorageActivities) Upload(ctx context.Context, req UploadFileRequest) (*UploadFileResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Uploading file", "bucket", req.Bucket, "path", req.Path, "size", len(req.Content))

	opts := storage.UploadOptions{
		Bucket:      req.Bucket,
		Path:        req.Path,
		ContentType: req.ContentType,
		CustomMeta:  req.Metadata,
		Public:      req.Public,
	}

	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}

	if req.TTLSeconds > 0 {
		opts.TTL = time.Duration(req.TTLSeconds) * time.Second
	}

	activity.RecordHeartbeat(ctx, "uploading file")

	meta, err := a.store.Upload(ctx, bytes.NewReader(req.Content), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	logger.Info("File uploaded successfully", "id", meta.ID, "path", meta.Path)

	return &UploadFileResult{
		ID:          meta.ID,
		Path:        meta.Path,
		Bucket:      meta.Bucket,
		Size:        meta.Size,
		ContentType: meta.ContentType,
		Checksum:    meta.Checksum,
		CreatedAt:   meta.CreatedAt,
	}, nil
}

// DownloadFileRequest represents a request to download a file.
type DownloadFileRequest struct {
	Bucket     string `json:"bucket"`
	Path       string `json:"path"`
	RangeStart int64  `json:"range_start,omitempty"`
	RangeEnd   int64  `json:"range_end,omitempty"`
}

// DownloadFileResult represents the result of downloading a file.
type DownloadFileResult struct {
	Content     []byte `json:"content"`
	Path        string `json:"path"`
	Bucket      string `json:"bucket"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

// Download downloads a file from cloud storage.
func (a *StorageActivities) Download(ctx context.Context, req DownloadFileRequest) (*DownloadFileResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Downloading file", "bucket", req.Bucket, "path", req.Path)

	opts := storage.DownloadOptions{
		Bucket: req.Bucket,
		Path:   req.Path,
	}

	if req.RangeStart > 0 || req.RangeEnd > 0 {
		opts.Range = &storage.ByteRange{
			Start: req.RangeStart,
			End:   req.RangeEnd,
		}
	}

	activity.RecordHeartbeat(ctx, "downloading file")

	reader, meta, err := a.store.Download(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	logger.Info("File downloaded successfully", "path", req.Path, "size", len(content))

	return &DownloadFileResult{
		Content:     content,
		Path:        meta.Path,
		Bucket:      meta.Bucket,
		Size:        int64(len(content)),
		ContentType: meta.ContentType,
	}, nil
}

// DeleteFileRequest represents a request to delete a file.
type DeleteStorageFileRequest struct {
	Bucket string `json:"bucket"`
	Path   string `json:"path"`
}

// DeleteStorageFile deletes a file from cloud storage.
func (a *StorageActivities) DeleteStorageFile(ctx context.Context, req DeleteStorageFileRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Deleting file", "bucket", req.Bucket, "path", req.Path)

	if err := a.store.Delete(ctx, req.Bucket, req.Path); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	logger.Info("File deleted successfully")
	return nil
}

// GetFileMetadataRequest represents a request to get file metadata.
type GetFileMetadataRequest struct {
	Bucket string `json:"bucket"`
	Path   string `json:"path"`
}

// GetFileMetadataResult represents file metadata.
type GetFileMetadataResult struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Bucket      string            `json:"bucket"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type"`
	Checksum    string            `json:"checksum,omitempty"`
	ETag        string            `json:"etag,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// GetMetadata retrieves file metadata without downloading.
func (a *StorageActivities) GetMetadata(ctx context.Context, req GetFileMetadataRequest) (*GetFileMetadataResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Getting file metadata", "bucket", req.Bucket, "path", req.Path)

	meta, err := a.store.GetMetadata(ctx, req.Bucket, req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return &GetFileMetadataResult{
		ID:          meta.ID,
		Name:        meta.Name,
		Path:        meta.Path,
		Bucket:      meta.Bucket,
		Size:        meta.Size,
		ContentType: meta.ContentType,
		Checksum:    meta.Checksum,
		ETag:        meta.ETag,
		CreatedAt:   meta.CreatedAt,
		UpdatedAt:   meta.UpdatedAt,
		Metadata:    meta.CustomMeta,
	}, nil
}

// GenerateUploadURLRequest represents a request to generate an upload URL.
type GenerateUploadURLRequest struct {
	Bucket           string `json:"bucket"`
	Path             string `json:"path,omitempty"`
	ContentType      string `json:"content_type,omitempty"`
	ExpirationSeconds int   `json:"expiration_seconds,omitempty"`
}

// GenerateURLResult represents a signed URL result.
type GenerateURLResult struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	ExpiresAt time.Time         `json:"expires_at"`
	Headers   map[string]string `json:"headers,omitempty"`
	Path      string            `json:"path"` // The path that will be used
}

// GenerateUploadURL generates a presigned URL for uploading.
func (a *StorageActivities) GenerateUploadURL(ctx context.Context, req GenerateUploadURLRequest) (*GenerateURLResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Generating upload URL", "bucket", req.Bucket)

	expiration := time.Hour
	if req.ExpirationSeconds > 0 {
		expiration = time.Duration(req.ExpirationSeconds) * time.Second
	}

	opts := storage.SignedURLOptions{
		Bucket:      req.Bucket,
		Path:        req.Path,
		ContentType: req.ContentType,
		Expiration:  expiration,
		Method:      "PUT",
	}

	result, err := a.store.GenerateUploadURL(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload URL: %w", err)
	}

	path := req.Path
	if path == "" && result.URL != "" {
		// Extract path from URL for local storage
		path = opts.Path
	}

	return &GenerateURLResult{
		URL:       result.URL,
		Method:    result.Method,
		ExpiresAt: result.ExpiresAt,
		Headers:   result.Headers,
		Path:      path,
	}, nil
}

// GenerateDownloadURLRequest represents a request to generate a download URL.
type GenerateDownloadURLRequest struct {
	Bucket            string `json:"bucket"`
	Path              string `json:"path"`
	ExpirationSeconds int    `json:"expiration_seconds,omitempty"`
}

// GenerateDownloadURL generates a presigned URL for downloading.
func (a *StorageActivities) GenerateDownloadURL(ctx context.Context, req GenerateDownloadURLRequest) (*GenerateURLResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Generating download URL", "bucket", req.Bucket, "path", req.Path)

	expiration := time.Hour
	if req.ExpirationSeconds > 0 {
		expiration = time.Duration(req.ExpirationSeconds) * time.Second
	}

	opts := storage.SignedURLOptions{
		Bucket:     req.Bucket,
		Path:       req.Path,
		Expiration: expiration,
		Method:     "GET",
	}

	result, err := a.store.GenerateDownloadURL(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate download URL: %w", err)
	}

	return &GenerateURLResult{
		URL:       result.URL,
		Method:    result.Method,
		ExpiresAt: result.ExpiresAt,
		Headers:   result.Headers,
		Path:      req.Path,
	}, nil
}

// ListStorageFilesRequest represents a request to list files.
type ListStorageFilesRequest struct {
	Bucket     string `json:"bucket"`
	Prefix     string `json:"prefix,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Recursive  bool   `json:"recursive,omitempty"`
}

// ListStorageFilesResult represents the result of listing files.
type ListStorageFilesResult struct {
	Files       []GetFileMetadataResult `json:"files"`
	Prefixes    []string                `json:"prefixes,omitempty"`
	NextCursor  string                  `json:"next_cursor,omitempty"`
	IsTruncated bool                    `json:"is_truncated"`
}

// ListStorageFiles lists files in cloud storage.
func (a *StorageActivities) ListStorageFiles(ctx context.Context, req ListStorageFilesRequest) (*ListStorageFilesResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing files", "bucket", req.Bucket, "prefix", req.Prefix)

	maxResults := 1000
	if req.MaxResults > 0 {
		maxResults = req.MaxResults
	}

	opts := storage.ListOptions{
		Bucket:     req.Bucket,
		Prefix:     req.Prefix,
		MaxResults: maxResults,
		StartAfter: req.Cursor,
		Recursive:  req.Recursive,
	}

	result, err := a.store.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	files := make([]GetFileMetadataResult, len(result.Files))
	for i, f := range result.Files {
		files[i] = GetFileMetadataResult{
			ID:          f.ID,
			Name:        f.Name,
			Path:        f.Path,
			Bucket:      f.Bucket,
			Size:        f.Size,
			ContentType: f.ContentType,
			ETag:        f.ETag,
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
		}
	}

	return &ListStorageFilesResult{
		Files:       files,
		Prefixes:    result.Prefixes,
		NextCursor:  result.NextCursor,
		IsTruncated: result.IsTruncated,
	}, nil
}

// CopyStorageFileRequest represents a request to copy a file.
type CopyStorageFileRequest struct {
	SourceBucket      string `json:"source_bucket"`
	SourcePath        string `json:"source_path"`
	DestinationBucket string `json:"destination_bucket"`
	DestinationPath   string `json:"destination_path"`
}

// CopyStorageFile copies a file within cloud storage.
func (a *StorageActivities) CopyStorageFile(ctx context.Context, req CopyStorageFileRequest) (*GetFileMetadataResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Copying file",
		"source", fmt.Sprintf("%s/%s", req.SourceBucket, req.SourcePath),
		"destination", fmt.Sprintf("%s/%s", req.DestinationBucket, req.DestinationPath))

	meta, err := a.store.Copy(ctx, req.SourceBucket, req.SourcePath, req.DestinationBucket, req.DestinationPath)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	return &GetFileMetadataResult{
		ID:        meta.ID,
		Name:      meta.Name,
		Path:      meta.Path,
		Bucket:    meta.Bucket,
		Size:      meta.Size,
		Checksum:  meta.Checksum,
		ETag:      meta.ETag,
		CreatedAt: meta.CreatedAt,
		UpdatedAt: meta.UpdatedAt,
	}, nil
}

// CreateBucketRequest represents a request to create a bucket.
type CreateBucketRequest struct {
	Bucket string `json:"bucket"`
}

// CreateBucket creates a new bucket.
func (a *StorageActivities) CreateBucket(ctx context.Context, req CreateBucketRequest) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Creating bucket", "bucket", req.Bucket)

	if err := a.store.CreateBucket(ctx, req.Bucket); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	logger.Info("Bucket created successfully")
	return nil
}

// ListBucketsResult represents the result of listing buckets.
type ListBucketsResult struct {
	Buckets []string `json:"buckets"`
}

// ListBuckets lists all buckets.
func (a *StorageActivities) ListBuckets(ctx context.Context) (*ListBucketsResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing buckets")

	buckets, err := a.store.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	return &ListBucketsResult{Buckets: buckets}, nil
}

// FileExistsRequest represents a request to check if a file exists.
type FileExistsRequest struct {
	Bucket string `json:"bucket"`
	Path   string `json:"path"`
}

// FileExistsResult represents the result of checking if a file exists.
type FileExistsResult struct {
	Exists bool `json:"exists"`
}

// FileExists checks if a file exists.
func (a *StorageActivities) FileExists(ctx context.Context, req FileExistsRequest) (*FileExistsResult, error) {
	exists, err := a.store.Exists(ctx, req.Bucket, req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to check file existence: %w", err)
	}
	return &FileExistsResult{Exists: exists}, nil
}
