package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalAttachmentStorageStoresFileAtDeterministicPath(t *testing.T) {
	root := t.TempDir()
	storage, err := newLocalAttachmentStorage(root)
	if err != nil {
		t.Fatalf("newLocalAttachmentStorage returned error: %v", err)
	}

	stored, err := storage.Store(AttachmentStorageKey{
		ProjectID:      7,
		FormXMLID:      "form/name",
		SubmissionUUID: "uuid:submission/id",
		Filename:       "photo 1.jpg",
	}, strings.NewReader("image bytes"))
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	expectedRelativePath := "project-7/form-form%2Fname/submission-submission%2Fid/photo%201.jpg"
	if stored.RelativePath != expectedRelativePath {
		t.Fatalf("expected relative path %q, got %q", expectedRelativePath, stored.RelativePath)
	}
	if stored.SizeBytes != int64(len("image bytes")) {
		t.Fatalf("unexpected stored size %d", stored.SizeBytes)
	}

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(stored.RelativePath)))
	if err != nil {
		t.Fatalf("failed to read stored attachment: %v", err)
	}
	if string(content) != "image bytes" {
		t.Fatalf("unexpected stored content %q", content)
	}
}

func TestLocalAttachmentStorageKeepsTraversalLikeNamesInsideRoot(t *testing.T) {
	root := t.TempDir()
	storage, err := newLocalAttachmentStorage(root)
	if err != nil {
		t.Fatalf("newLocalAttachmentStorage returned error: %v", err)
	}

	stored, err := storage.Store(AttachmentStorageKey{
		ProjectID:      7,
		FormXMLID:      "../form",
		SubmissionUUID: "uuid:../submission",
		Filename:       "../../photo.jpg",
	}, strings.NewReader("safe"))
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	targetPath := filepath.Join(root, filepath.FromSlash(stored.RelativePath))
	if err := ensurePathWithinRoot(root, targetPath); err != nil {
		t.Fatalf("stored path escaped root: %v", err)
	}
	if strings.Contains(stored.RelativePath, "/../") {
		t.Fatalf("stored path contains a traversal segment: %q", stored.RelativePath)
	}
}

func TestLocalAttachmentStorageAtomicallyReplacesExistingFile(t *testing.T) {
	root := t.TempDir()
	storage, err := newLocalAttachmentStorage(root)
	if err != nil {
		t.Fatalf("newLocalAttachmentStorage returned error: %v", err)
	}
	key := AttachmentStorageKey{ProjectID: 7, FormXMLID: "form", SubmissionUUID: "submission", Filename: "photo.jpg"}

	if _, err := storage.Store(key, strings.NewReader("old")); err != nil {
		t.Fatalf("first Store returned error: %v", err)
	}
	stored, err := storage.Store(key, strings.NewReader("new content"))
	if err != nil {
		t.Fatalf("second Store returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(stored.RelativePath)))
	if err != nil {
		t.Fatalf("failed to read replaced attachment: %v", err)
	}
	if string(content) != "new content" {
		t.Fatalf("unexpected replaced content %q", content)
	}
}

func TestLocalAttachmentStorageRemovesTemporaryFileAfterWriteFailure(t *testing.T) {
	root := t.TempDir()
	storage, err := newLocalAttachmentStorage(root)
	if err != nil {
		t.Fatalf("newLocalAttachmentStorage returned error: %v", err)
	}
	key := AttachmentStorageKey{ProjectID: 7, FormXMLID: "form", SubmissionUUID: "submission", Filename: "photo.jpg"}

	_, err = storage.Store(key, errorReader{})
	if err == nil || !strings.Contains(err.Error(), "failed to write attachment file") {
		t.Fatalf("expected write error, got %v", err)
	}

	directory := filepath.Join(root, "project-7", "form-form", "submission-submission")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("failed to read attachment directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no partial files, got %d", len(entries))
	}
}

func TestNewLocalAttachmentStorageRejectsEmptyRoot(t *testing.T) {
	_, err := newLocalAttachmentStorage("  ")
	if err == nil {
		t.Fatalf("expected empty root error")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

var _ io.Reader = errorReader{}
