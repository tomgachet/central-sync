package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func useTempWorkingDir(t *testing.T) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})
}

func TestDoJSONDecodesSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("expected Authorization token, got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected JSON content type, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer server.Close()

	client := &CentralClient{Token: "token", HTTPClient: server.Client()}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.DoJSON(http.MethodPost, server.URL, map[string]string{"name": "value"}, &out); err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected decoded OK response")
	}
}

func TestDoJSONReturnsErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "central failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &CentralClient{Token: "token", HTTPClient: server.Client()}

	err := client.DoJSON(http.MethodPatch, server.URL, nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "500 Internal Server Error") || !strings.Contains(err.Error(), "central failed") {
		t.Fatalf("expected status and body in error, got %v", err)
	}
}

func TestDoJSONRefreshesTokenAfterUnauthorized(t *testing.T) {
	useTempWorkingDir(t)

	var requestCount int
	var sessionCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sessions":
			sessionCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(sessionResponse{Token: "fresh-token", ExpiresAt: "2099-01-01T00:00:00Z"})
		case "/resource":
			requestCount++
			if requestCount == 1 {
				if got := r.Header.Get("Authorization"); got != "Bearer stale-token" {
					t.Fatalf("expected first request to use stale token, got %q", got)
				}
				http.Error(w, "expired", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer fresh-token" {
				t.Fatalf("expected retry to use fresh token, got %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("ODK_CENTRAL_URL", server.URL)
	t.Setenv("ODK_CENTRAL_USER_EMAIL", "user@example.com")
	t.Setenv("ODK_CENTRAL_USER_PASSWORD", "secret")

	client := &CentralClient{Token: "stale-token", HTTPClient: server.Client()}

	if err := client.DoJSON(http.MethodPatch, server.URL+"/resource", nil, nil); err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 resource requests, got %d", requestCount)
	}
	if sessionCount != 1 {
		t.Fatalf("expected 1 session request, got %d", sessionCount)
	}
	if client.Token != "fresh-token" {
		t.Fatalf("expected client token to be refreshed, got %q", client.Token)
	}
}

func TestDoJSONReturnsErrorWhenRetryIsStillUnauthorized(t *testing.T) {
	useTempWorkingDir(t)

	var requestCount int
	var sessionCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sessions":
			sessionCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(sessionResponse{Token: "fresh-token", ExpiresAt: "2099-01-01T00:00:00Z"})
		case "/resource":
			requestCount++
			http.Error(w, "still unauthorized", http.StatusUnauthorized)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("ODK_CENTRAL_URL", server.URL)
	t.Setenv("ODK_CENTRAL_USER_EMAIL", "user@example.com")
	t.Setenv("ODK_CENTRAL_USER_PASSWORD", "secret")

	client := &CentralClient{Token: "stale-token", HTTPClient: server.Client()}

	err := client.DoJSON(http.MethodPost, server.URL+"/resource", nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") || !strings.Contains(err.Error(), "still unauthorized") {
		t.Fatalf("expected retry 401 error with body, got %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 resource requests, got %d", requestCount)
	}
	if sessionCount != 1 {
		t.Fatalf("expected 1 session request, got %d", sessionCount)
	}
}
