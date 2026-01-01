package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// S3Config contains configuration for S3-compatible storage.
type S3Config struct {
	Region          string
	Endpoint        string // Custom endpoint for MinIO/other S3-compatible
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool // Required for MinIO
	DisableSSL      bool // For local development
}

// S3Store implements file storage on S3-compatible services.
type S3Store struct {
	client      *s3.Client
	presignClient *s3.PresignClient
	config      S3Config
	storageType StorageType
}

// NewS3Store creates a new S3 store.
func NewS3Store(cfg S3Config) (*S3Store, error) {
	var awsCfg aws.Config
	var err error

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err = config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = cfg.UsePathStyle
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)
	presignClient := s3.NewPresignClient(client)

	storageType := StorageTypeS3
	if cfg.Endpoint != "" {
		storageType = StorageTypeMinIO
	}

	return &S3Store{
		client:        client,
		presignClient: presignClient,
		config:        cfg,
		storageType:   storageType,
	}, nil
}

// Upload uploads a file to S3.
func (s *S3Store) Upload(ctx context.Context, reader io.Reader, opts UploadOptions) (*FileMetadata, error) {
	path := opts.Path
	if path == "" {
		path = uuid.New().String()
	}

	// Read all content to calculate hash and size
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	input := &s3.PutObjectInput{
		Bucket:      aws.String(opts.Bucket),
		Key:         aws.String(path),
		Body:        bytes.NewReader(content),
		ContentType: aws.String(opts.ContentType),
	}

	// Add custom metadata
	if len(opts.CustomMeta) > 0 {
		input.Metadata = opts.CustomMeta
	}

	// Add storage class
	if opts.StorageClass != "" {
		input.StorageClass = types.StorageClass(opts.StorageClass)
	}

	// Make public if requested
	if opts.Public {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	result, err := s.client.PutObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}

	now := time.Now()
	etag := ""
	if result.ETag != nil {
		etag = *result.ETag
	}

	return &FileMetadata{
		ID:           uuid.New().String(),
		Name:         path,
		Path:         path,
		Bucket:       opts.Bucket,
		Size:         int64(len(content)),
		ContentType:  opts.ContentType,
		Checksum:     checksum,
		ETag:         etag,
		CreatedAt:    now,
		UpdatedAt:    now,
		CustomMeta:   opts.CustomMeta,
		StorageClass: opts.StorageClass,
	}, nil
}

// Download downloads a file from S3.
func (s *S3Store) Download(ctx context.Context, opts DownloadOptions) (io.ReadCloser, *FileMetadata, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(opts.Bucket),
		Key:    aws.String(opts.Path),
	}

	if opts.Range != nil {
		input.Range = aws.String(fmt.Sprintf("bytes=%d-%d", opts.Range.Start, opts.Range.End))
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download: %w", err)
	}

	meta := &FileMetadata{
		Path:        opts.Path,
		Bucket:      opts.Bucket,
		Size:        aws.ToInt64(result.ContentLength),
		ContentType: aws.ToString(result.ContentType),
		ETag:        aws.ToString(result.ETag),
	}

	if result.LastModified != nil {
		meta.UpdatedAt = *result.LastModified
	}

	return result.Body, meta, nil
}

// Delete deletes a file from S3.
func (s *S3Store) Delete(ctx context.Context, bucket, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}
	return nil
}

// GetMetadata retrieves file metadata.
func (s *S3Store) GetMetadata(ctx context.Context, bucket, path string) (*FileMetadata, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	meta := &FileMetadata{
		Path:         path,
		Bucket:       bucket,
		Size:         aws.ToInt64(result.ContentLength),
		ContentType:  aws.ToString(result.ContentType),
		ETag:         aws.ToString(result.ETag),
		CustomMeta:   result.Metadata,
		StorageClass: string(result.StorageClass),
	}

	if result.LastModified != nil {
		meta.UpdatedAt = *result.LastModified
		meta.CreatedAt = *result.LastModified
	}

	return meta, nil
}

// Exists checks if a file exists.
func (s *S3Store) Exists(ctx context.Context, bucket, path string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		// Check if it's a not found error
		return false, nil
	}
	return true, nil
}

// List lists files in S3.
func (s *S3Store) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(opts.Bucket),
		MaxKeys: aws.Int32(int32(opts.MaxResults)),
	}

	if opts.Prefix != "" {
		input.Prefix = aws.String(opts.Prefix)
	}

	if !opts.Recursive && opts.Delimiter == "" {
		input.Delimiter = aws.String("/")
	} else if opts.Delimiter != "" {
		input.Delimiter = aws.String(opts.Delimiter)
	}

	if opts.StartAfter != "" {
		input.StartAfter = aws.String(opts.StartAfter)
	}

	result, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	listResult := &ListResult{
		Files:       make([]FileMetadata, 0, len(result.Contents)),
		Prefixes:    make([]string, 0, len(result.CommonPrefixes)),
		IsTruncated: aws.ToBool(result.IsTruncated),
	}

	if result.NextContinuationToken != nil {
		listResult.NextCursor = *result.NextContinuationToken
	}

	for _, obj := range result.Contents {
		meta := FileMetadata{
			Path:         aws.ToString(obj.Key),
			Bucket:       opts.Bucket,
			Size:         aws.ToInt64(obj.Size),
			ETag:         aws.ToString(obj.ETag),
			StorageClass: string(obj.StorageClass),
		}
		if obj.LastModified != nil {
			meta.UpdatedAt = *obj.LastModified
			meta.CreatedAt = *obj.LastModified
		}
		listResult.Files = append(listResult.Files, meta)
	}

	for _, prefix := range result.CommonPrefixes {
		listResult.Prefixes = append(listResult.Prefixes, aws.ToString(prefix.Prefix))
	}

	return listResult, nil
}

// GenerateUploadURL generates a presigned URL for uploading.
func (s *S3Store) GenerateUploadURL(ctx context.Context, opts SignedURLOptions) (*SignedURL, error) {
	path := opts.Path
	if path == "" {
		path = uuid.New().String()
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(opts.Bucket),
		Key:    aws.String(path),
	}

	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}

	presignResult, err := s.presignClient.PresignPutObject(ctx, input, func(po *s3.PresignOptions) {
		po.Expires = opts.Expiration
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload URL: %w", err)
	}

	return &SignedURL{
		URL:       presignResult.URL,
		Method:    presignResult.Method,
		ExpiresAt: time.Now().Add(opts.Expiration),
		Headers:   httpHeaderToMap(presignResult.SignedHeader),
	}, nil
}

// GenerateDownloadURL generates a presigned URL for downloading.
func (s *S3Store) GenerateDownloadURL(ctx context.Context, opts SignedURLOptions) (*SignedURL, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(opts.Bucket),
		Key:    aws.String(opts.Path),
	}

	presignResult, err := s.presignClient.PresignGetObject(ctx, input, func(po *s3.PresignOptions) {
		po.Expires = opts.Expiration
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate download URL: %w", err)
	}

	return &SignedURL{
		URL:       presignResult.URL,
		Method:    presignResult.Method,
		ExpiresAt: time.Now().Add(opts.Expiration),
		Headers:   httpHeaderToMap(presignResult.SignedHeader),
	}, nil
}

// httpHeaderToMap converts http.Header to map[string]string.
func httpHeaderToMap(header map[string][]string) map[string]string {
	result := make(map[string]string)
	for k, v := range header {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// Copy copies a file within S3.
func (s *S3Store) Copy(ctx context.Context, srcBucket, srcPath, dstBucket, dstPath string) (*FileMetadata, error) {
	copySource := fmt.Sprintf("%s/%s", srcBucket, srcPath)

	result, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstPath),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to copy: %w", err)
	}

	meta := &FileMetadata{
		ID:     uuid.New().String(),
		Path:   dstPath,
		Bucket: dstBucket,
		ETag:   aws.ToString(result.CopyObjectResult.ETag),
	}

	if result.CopyObjectResult.LastModified != nil {
		meta.UpdatedAt = *result.CopyObjectResult.LastModified
		meta.CreatedAt = *result.CopyObjectResult.LastModified
	}

	return meta, nil
}

// CreateBucket creates a new bucket.
func (s *S3Store) CreateBucket(ctx context.Context, bucket string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	// Only set LocationConstraint if not us-east-1
	if s.config.Region != "" && s.config.Region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(s.config.Region),
		}
	}

	_, err := s.client.CreateBucket(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}
	return nil
}

// DeleteBucket deletes a bucket.
func (s *S3Store) DeleteBucket(ctx context.Context, bucket string) error {
	_, err := s.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}
	return nil
}

// ListBuckets lists all buckets.
func (s *S3Store) ListBuckets(ctx context.Context) ([]string, error) {
	result, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	buckets := make([]string, len(result.Buckets))
	for i, b := range result.Buckets {
		buckets[i] = aws.ToString(b.Name)
	}

	return buckets, nil
}

// Health checks if the store is healthy.
func (s *S3Store) Health(ctx context.Context) error {
	_, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	return err
}

// Close closes the store.
func (s *S3Store) Close() error {
	return nil
}

// Type returns the storage type.
func (s *S3Store) Type() StorageType {
	return s.storageType
}
