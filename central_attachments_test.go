package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListSubmissionAttachments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/projects/7/forms/form%2Fname/submissions/uuid:submission%2Fid/attachments" {
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"name":"photo 1.jpg","exists":true},{"name":"missing.jpg","exists":false}]`)
	}))
	defer server.Close()

	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	attachments, err := listSubmissionAttachments(client, 7, "form/name", "uuid:submission/id")
	if err != nil {
		t.Fatalf("listSubmissionAttachments returned error: %v", err)
	}
	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachments))
	}
	if attachments[0].Name != "photo 1.jpg" || !attachments[0].Exists {
		t.Fatalf("unexpected first attachment: %#v", attachments[0])
	}
	if attachments[1].Exists {
		t.Fatalf("expected second attachment to be missing")
	}
}

func TestListSubmissionAttachmentsReturnsEndpointError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "submission not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	_, err := listSubmissionAttachments(client, 7, "form", "uuid:missing")
	if err == nil || !strings.Contains(err.Error(), "404 Not Found") || !strings.Contains(err.Error(), "submission not found") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestDownloadSubmissionAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/projects/7/forms/form/submissions/uuid:submission/attachments/photo%201.jpg" {
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Fatalf("unexpected Accept header %q", got)
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", `"media-version"`)
		_, _ = io.WriteString(w, "image bytes")
	}))
	defer server.Close()

	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	resp, err := downloadSubmissionAttachment(client, 7, "form", "uuid:submission", "photo 1.jpg")
	if err != nil {
		t.Fatalf("downloadSubmissionAttachment returned error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read attachment body: %v", err)
	}
	if string(body) != "image bytes" {
		t.Fatalf("unexpected attachment body %q", body)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("unexpected content type %q", got)
	}
	if got := resp.Header.Get("ETag"); got != `"media-version"` {
		t.Fatalf("unexpected ETag %q", got)
	}
}

func TestDownloadSubmissionAttachmentReturnsEndpointError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "attachment missing", http.StatusNotFound)
	}))
	defer server.Close()

	client := &CentralClient{BaseURL: server.URL, Token: "token", HTTPClient: server.Client()}
	_, err := downloadSubmissionAttachment(client, 7, "form", "uuid:submission", "missing.jpg")
	if err == nil || !strings.Contains(err.Error(), "404 Not Found") || !strings.Contains(err.Error(), "attachment missing") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}
