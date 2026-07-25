package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncSubmissionAttachmentsStoresPresentAttachments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/v1/projects/7/forms/form/submissions/uuid:submission/attachments":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[{"name":"photo.jpg","exists":true},{"name":"missing.jpg","exists":false}]`)
		case "/v1/projects/7/forms/form/submissions/uuid:submission/attachments/photo.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("ETag", `"photo-v1"`)
			_, _ = io.WriteString(w, "image bytes")
		default:
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	storage := &recordingAttachmentStorage{}
	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	result, err := syncSubmissionAttachments(client, storage, 7, "form", "submission")
	if err != nil {
		t.Fatalf("syncSubmissionAttachments returned error: %v", err)
	}

	if result.Expected != 2 || result.Present != 1 || len(result.Stored) != 1 || len(result.Missing) != 1 {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if result.Missing[0] != "missing.jpg" {
		t.Fatalf("unexpected missing attachments: %#v", result.Missing)
	}
	if len(storage.calls) != 1 {
		t.Fatalf("expected 1 storage call, got %d", len(storage.calls))
	}
	if storage.calls[0].key.Filename != "photo.jpg" || storage.calls[0].content != "image bytes" {
		t.Fatalf("unexpected storage call: %#v", storage.calls[0])
	}
	if result.Stored[0].ContentType != "image/jpeg" || result.Stored[0].ETag != `"photo-v1"` {
		t.Fatalf("unexpected stored metadata: %#v", result.Stored[0])
	}
}

func TestSyncSubmissionAttachmentsAcceptsPrefixedInstanceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/projects/7/forms/form/submissions/uuid:submission/attachments" {
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer server.Close()

	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	if _, err := syncSubmissionAttachments(client, &recordingAttachmentStorage{}, 7, "form", "uuid:submission"); err != nil {
		t.Fatalf("syncSubmissionAttachments returned error: %v", err)
	}
}

func TestSyncSubmissionAttachmentsReturnsDownloadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/attachments") {
			_, _ = io.WriteString(w, `[{"name":"photo.jpg","exists":true}]`)
			return
		}
		http.Error(w, "download failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	result, err := syncSubmissionAttachments(client, &recordingAttachmentStorage{}, 7, "form", "submission")
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("expected download error, got %v", err)
	}
	if result == nil || result.Expected != 1 || result.Present != 1 || len(result.Stored) != 0 {
		t.Fatalf("unexpected partial result: %#v", result)
	}
}

func TestSyncSubmissionAttachmentsReturnsStorageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/attachments") {
			_, _ = io.WriteString(w, `[{"name":"photo.jpg","exists":true}]`)
			return
		}
		_, _ = io.WriteString(w, "image bytes")
	}))
	defer server.Close()

	storage := &recordingAttachmentStorage{err: errors.New("disk full")}
	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	_, err := syncSubmissionAttachments(client, storage, 7, "form", "submission")
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected storage error, got %v", err)
	}
}

type attachmentStorageCall struct {
	key     AttachmentStorageKey
	content string
}

type recordingAttachmentStorage struct {
	calls []attachmentStorageCall
	err   error
}

func (s *recordingAttachmentStorage) Store(key AttachmentStorageKey, source io.Reader) (*StoredAttachment, error) {
	content, err := io.ReadAll(source)
	if err != nil {
		return nil, err
	}
	s.calls = append(s.calls, attachmentStorageCall{key: key, content: string(content)})
	if s.err != nil {
		return nil, s.err
	}
	return &StoredAttachment{RelativePath: "stored/" + key.Filename, SizeBytes: int64(len(content))}, nil
}
