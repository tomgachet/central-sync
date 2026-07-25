package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type AttachmentStorageKey struct {
	ProjectID      int
	FormXMLID      string
	SubmissionUUID string
	Filename       string
}

type StoredAttachment struct {
	RelativePath string
	SizeBytes    int64
}

type AttachmentStorage interface {
	Store(key AttachmentStorageKey, source io.Reader) (*StoredAttachment, error)
}

type LocalAttachmentStorage struct {
	rootDirectory string
}

func newLocalAttachmentStorage(rootDirectory string) (*LocalAttachmentStorage, error) {
	if strings.TrimSpace(rootDirectory) == "" {
		return nil, fmt.Errorf("attachment storage root directory is required")
	}

	absoluteRoot, err := filepath.Abs(rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve attachment storage root directory: %w", err)
	}

	return &LocalAttachmentStorage{rootDirectory: filepath.Clean(absoluteRoot)}, nil
}

func (s *LocalAttachmentStorage) Store(key AttachmentStorageKey, source io.Reader) (*StoredAttachment, error) {
	if s == nil || s.rootDirectory == "" {
		return nil, fmt.Errorf("local attachment storage is not initialized")
	}
	if source == nil {
		return nil, fmt.Errorf("attachment source is required")
	}
	if key.ProjectID <= 0 {
		return nil, fmt.Errorf("attachment project ID must be greater than 0")
	}
	if strings.TrimSpace(key.FormXMLID) == "" {
		return nil, fmt.Errorf("attachment form XML ID is required")
	}
	if strings.TrimSpace(key.SubmissionUUID) == "" {
		return nil, fmt.Errorf("attachment submission UUID is required")
	}
	if strings.TrimSpace(key.Filename) == "" {
		return nil, fmt.Errorf("attachment filename is required")
	}

	relativePath := filepath.Join(
		fmt.Sprintf("project-%d", key.ProjectID),
		"form-"+encodeStoragePathComponent(key.FormXMLID),
		"submission-"+encodeStoragePathComponent(trimUUIDPrefix(key.SubmissionUUID)),
		encodeStoragePathComponent(key.Filename),
	)
	targetPath := filepath.Join(s.rootDirectory, relativePath)

	if err := ensurePathWithinRoot(s.rootDirectory, targetPath); err != nil {
		return nil, err
	}

	targetDirectory := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDirectory, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create attachment directory: %w", err)
	}

	temporaryFile, err := os.CreateTemp(targetDirectory, ".central-sync-attachment-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary attachment file: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	keepTemporaryFile := false
	defer func() {
		if !keepTemporaryFile {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporaryFile.Chmod(0o640); err != nil {
		_ = temporaryFile.Close()
		return nil, fmt.Errorf("failed to set attachment file permissions: %w", err)
	}

	sizeBytes, err := io.Copy(temporaryFile, source)
	if err != nil {
		_ = temporaryFile.Close()
		return nil, fmt.Errorf("failed to write attachment file: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		_ = temporaryFile.Close()
		return nil, fmt.Errorf("failed to flush attachment file: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close attachment file: %w", err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return nil, fmt.Errorf("failed to publish attachment file: %w", err)
	}
	keepTemporaryFile = true

	return &StoredAttachment{
		RelativePath: filepath.ToSlash(relativePath),
		SizeBytes:    sizeBytes,
	}, nil
}

func encodeStoragePathComponent(value string) string {
	escaped := url.PathEscape(value)
	return strings.ReplaceAll(escaped, `\`, "%5C")
}

func ensurePathWithinRoot(rootDirectory string, targetPath string) error {
	relativePath, err := filepath.Rel(rootDirectory, targetPath)
	if err != nil {
		return fmt.Errorf("failed to validate attachment path: %w", err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return fmt.Errorf("attachment path escapes storage root")
	}
	return nil
}
