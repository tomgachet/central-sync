package main

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredAttachmentStorageReturnsNilWhenDisabled(t *testing.T) {
	storage, err := configuredAttachmentStorage([]ProjectMapping{{
		Forms: []FormMapping{{Sync: true, SyncAttachments: false}},
	}}, AttachmentStorageConfig{})
	if err != nil {
		t.Fatalf("configuredAttachmentStorage returned error: %v", err)
	}
	if storage != nil {
		t.Fatalf("expected nil storage when attachment sync is disabled")
	}
}

func TestConfiguredAttachmentStorageBuildsLocalBackend(t *testing.T) {
	root := t.TempDir()
	storage, err := configuredAttachmentStorage([]ProjectMapping{{
		Forms: []FormMapping{{Sync: true, SyncAttachments: true}},
	}}, AttachmentStorageConfig{Backend: StorageBackendLocal, LocalDirectory: root})
	if err != nil {
		t.Fatalf("configuredAttachmentStorage returned error: %v", err)
	}

	localStorage, ok := storage.(*LocalAttachmentStorage)
	if !ok {
		t.Fatalf("expected local attachment storage, got %T", storage)
	}
	expectedRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("failed to resolve expected root: %v", err)
	}
	if localStorage.rootDirectory != expectedRoot {
		t.Fatalf("expected storage root %q, got %q", expectedRoot, localStorage.rootDirectory)
	}
}

func TestSyncAndPersistSubmissionAttachments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/attachments") {
			_, _ = io.WriteString(w, `[{"name":"photo.jpg","exists":true},{"name":"cleared.jpg","exists":false}]`)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", `"photo-v1"`)
		_, _ = io.WriteString(w, "image bytes")
	}))
	defer server.Close()

	executor := &attachmentMetadataExecutor{}
	storage := &recordingAttachmentStorage{}
	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	result, err := syncAndPersistSubmissionAttachments(
		executor,
		client,
		storage,
		42,
		7,
		"site_visit",
		"uuid:123e4567-e89b-12d3-a456-426614174000",
	)
	if err != nil {
		t.Fatalf("syncAndPersistSubmissionAttachments returned error: %v", err)
	}

	if len(result.Stored) != 1 || len(result.Missing) != 1 || len(executor.calls) != 2 {
		t.Fatalf("expected one stored and one missing attachment metadata update, got result=%#v calls=%d", result, len(executor.calls))
	}
	args := executor.calls[0]
	if args[0] != 7 || args[2] != "123e4567-e89b-12d3-a456-426614174000" || args[3] != "photo.jpg" {
		t.Fatalf("unexpected metadata identity arguments: %#v", args)
	}
	if args[4] != StorageBackendLocal || args[6] != "image/jpeg" || args[9] != `"photo-v1"` || args[10] != int64(42) {
		t.Fatalf("unexpected storage metadata arguments: %#v", args)
	}
	missingArgs := executor.calls[1]
	if missingArgs[0] != 7 || missingArgs[2] != "123e4567-e89b-12d3-a456-426614174000" || missingArgs[3] != "cleared.jpg" || missingArgs[5] != int64(42) {
		t.Fatalf("unexpected missing metadata arguments: %#v", missingArgs)
	}
}

type attachmentMetadataExecutor struct {
	calls [][]interface{}
}

func (e *attachmentMetadataExecutor) Exec(_ string, args ...interface{}) (sql.Result, error) {
	call := append([]interface{}(nil), args...)
	e.calls = append(e.calls, call)
	return staticSQLResult(1), nil
}
