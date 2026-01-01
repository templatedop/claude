package activity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.temporal.io/sdk/activity"
)

// FileSystemActivities contains activities for file system operations.
type FileSystemActivities struct {
	workingDir string
	allowedDirs []string
}

// NewFileSystemActivities creates a new FileSystemActivities instance.
func NewFileSystemActivities(workingDir string, allowedDirs []string) *FileSystemActivities {
	return &FileSystemActivities{
		workingDir:  workingDir,
		allowedDirs: allowedDirs,
	}
}

// ReadFileRequest represents a request to read a file.
type ReadFileRequest struct {
	Path     string `json:"path"`
	Encoding string `json:"encoding,omitempty"` // default: utf-8
}

// ReadFileResult represents the result of reading a file.
type ReadFileResult struct {
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
	Path     string `json:"path"`
}

// ReadFile reads a file and returns its content.
func (a *FileSystemActivities) ReadFile(ctx context.Context, req ReadFileRequest) (*ReadFileResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Reading file", "path", req.Path)

	fullPath, err := a.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	if !a.isAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path %s is outside allowed directories", req.Path)
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", req.Path)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	hash := sha256.Sum256(content)

	return &ReadFileResult{
		Content:  string(content),
		Size:     info.Size(),
		Checksum: hex.EncodeToString(hash[:]),
		Path:     fullPath,
	}, nil
}

// WriteFileRequest represents a request to write a file.
type WriteFileRequest struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Mode      uint32 `json:"mode,omitempty"` // default: 0644
	Overwrite bool   `json:"overwrite"`
	CreateDir bool   `json:"create_dir"` // create parent directories if needed
}

// WriteFileResult represents the result of writing a file.
type WriteFileResult struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
	Created  bool   `json:"created"` // true if file was created, false if overwritten
}

// WriteFile writes content to a file.
func (a *FileSystemActivities) WriteFile(ctx context.Context, req WriteFileRequest) (*WriteFileResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Writing file", "path", req.Path)

	fullPath, err := a.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	if !a.isAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path %s is outside allowed directories", req.Path)
	}

	// Check if file exists
	_, err = os.Stat(fullPath)
	fileExists := err == nil
	if fileExists && !req.Overwrite {
		return nil, fmt.Errorf("file already exists and overwrite is false: %s", req.Path)
	}

	// Create parent directories if needed
	if req.CreateDir {
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directories: %w", err)
		}
	}

	mode := os.FileMode(req.Mode)
	if mode == 0 {
		mode = 0644
	}

	content := []byte(req.Content)
	if err := os.WriteFile(fullPath, content, mode); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	hash := sha256.Sum256(content)

	return &WriteFileResult{
		Path:     fullPath,
		Size:     int64(len(content)),
		Checksum: hex.EncodeToString(hash[:]),
		Created:  !fileExists,
	}, nil
}

// DeleteFileRequest represents a request to delete a file.
type DeleteFileRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"` // for directories
}

// DeleteFileResult represents the result of deleting a file.
type DeleteFileResult struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

// DeleteFile deletes a file or directory.
func (a *FileSystemActivities) DeleteFile(ctx context.Context, req DeleteFileRequest) (*DeleteFileResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Deleting file", "path", req.Path, "recursive", req.Recursive)

	fullPath, err := a.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	if !a.isAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path %s is outside allowed directories", req.Path)
	}

	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return &DeleteFileResult{Path: fullPath, Deleted: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		if req.Recursive {
			if err := os.RemoveAll(fullPath); err != nil {
				return nil, fmt.Errorf("failed to remove directory: %w", err)
			}
		} else {
			if err := os.Remove(fullPath); err != nil {
				return nil, fmt.Errorf("failed to remove directory (use recursive for non-empty): %w", err)
			}
		}
	} else {
		if err := os.Remove(fullPath); err != nil {
			return nil, fmt.Errorf("failed to remove file: %w", err)
		}
	}

	return &DeleteFileResult{Path: fullPath, Deleted: true}, nil
}

// ListFilesRequest represents a request to list files.
type ListFilesRequest struct {
	Path      string `json:"path"`
	Pattern   string `json:"pattern,omitempty"` // glob pattern
	Recursive bool   `json:"recursive"`
}

// FileInfo represents information about a file.
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// ListFilesResult represents the result of listing files.
type ListFilesResult struct {
	Files []FileInfo `json:"files"`
	Count int        `json:"count"`
}

// ListFiles lists files in a directory.
func (a *FileSystemActivities) ListFiles(ctx context.Context, req ListFilesRequest) (*ListFilesResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Listing files", "path", req.Path, "recursive", req.Recursive)

	fullPath, err := a.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	if !a.isAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path %s is outside allowed directories", req.Path)
	}

	var files []FileInfo

	if req.Recursive {
		err = filepath.WalkDir(fullPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Check context cancellation
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if req.Pattern != "" {
				matched, _ := filepath.Match(req.Pattern, d.Name())
				if !matched {
					return nil
				}
			}

			info, err := d.Info()
			if err != nil {
				return nil // Skip files we can't stat
			}

			files = append(files, FileInfo{
				Name:    d.Name(),
				Path:    path,
				Size:    info.Size(),
				IsDir:   d.IsDir(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})

			return nil
		})
	} else {
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			if req.Pattern != "" {
				matched, _ := filepath.Match(req.Pattern, entry.Name())
				if !matched {
					continue
				}
			}

			info, err := entry.Info()
			if err != nil {
				continue // Skip files we can't stat
			}

			files = append(files, FileInfo{
				Name:    entry.Name(),
				Path:    filepath.Join(fullPath, entry.Name()),
				Size:    info.Size(),
				IsDir:   entry.IsDir(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}

	if err != nil && err != context.Canceled {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return &ListFilesResult{
		Files: files,
		Count: len(files),
	}, nil
}

// CopyFileRequest represents a request to copy a file.
type CopyFileRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Overwrite   bool   `json:"overwrite"`
}

// CopyFileResult represents the result of copying a file.
type CopyFileResult struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Size        int64  `json:"size"`
}

// CopyFile copies a file from source to destination.
func (a *FileSystemActivities) CopyFile(ctx context.Context, req CopyFileRequest) (*CopyFileResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Copying file", "source", req.Source, "destination", req.Destination)

	srcPath, err := a.resolvePath(req.Source)
	if err != nil {
		return nil, err
	}

	dstPath, err := a.resolvePath(req.Destination)
	if err != nil {
		return nil, err
	}

	if !a.isAllowed(srcPath) || !a.isAllowed(dstPath) {
		return nil, fmt.Errorf("access denied: paths are outside allowed directories")
	}

	// Check destination
	if _, err := os.Stat(dstPath); err == nil && !req.Overwrite {
		return nil, fmt.Errorf("destination already exists and overwrite is false: %s", req.Destination)
	}

	// Open source
	src, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create destination
	dst, err := os.Create(dstPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination: %w", err)
	}
	defer dst.Close()

	// Copy content
	size, err := io.Copy(dst, src)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	return &CopyFileResult{
		Source:      srcPath,
		Destination: dstPath,
		Size:        size,
	}, nil
}

// CreateDirectoryRequest represents a request to create a directory.
type CreateDirectoryRequest struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode,omitempty"` // default: 0755
}

// CreateDirectoryResult represents the result of creating a directory.
type CreateDirectoryResult struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// CreateDirectory creates a directory.
func (a *FileSystemActivities) CreateDirectory(ctx context.Context, req CreateDirectoryRequest) (*CreateDirectoryResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Creating directory", "path", req.Path)

	fullPath, err := a.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	if !a.isAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path %s is outside allowed directories", req.Path)
	}

	mode := os.FileMode(req.Mode)
	if mode == 0 {
		mode = 0755
	}

	// Check if already exists
	if info, err := os.Stat(fullPath); err == nil {
		if info.IsDir() {
			return &CreateDirectoryResult{Path: fullPath, Created: false}, nil
		}
		return nil, fmt.Errorf("path exists but is not a directory: %s", req.Path)
	}

	if err := os.MkdirAll(fullPath, mode); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	return &CreateDirectoryResult{Path: fullPath, Created: true}, nil
}

// resolvePath resolves a path relative to the working directory.
func (a *FileSystemActivities) resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Abs(filepath.Join(a.workingDir, path))
}

// isAllowed checks if a path is within allowed directories.
func (a *FileSystemActivities) isAllowed(path string) bool {
	if len(a.allowedDirs) == 0 {
		return true // No restrictions
	}

	cleanPath := filepath.Clean(path)
	for _, allowed := range a.allowedDirs {
		cleanAllowed := filepath.Clean(allowed)
		if strings.HasPrefix(cleanPath, cleanAllowed) {
			return true
		}
	}
	return false
}
