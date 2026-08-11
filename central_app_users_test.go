package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListProjectAppUsersRequestsExtendedMetadataAndDiscardsTokens(t *testing.T) {
	const secretToken = "secret-app-user-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/7/app-users" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Extended-Metadata"); got != "true" {
			t.Fatalf("expected X-Extended-Metadata header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": 115,
				"projectId": 7,
				"displayName": "Collector North",
				"type": "field_key",
				"createdAt": "2026-07-01T10:00:00Z",
				"updatedAt": "2026-07-02T11:00:00Z",
				"lastUsed": "2026-07-03T12:00:00Z",
				"token": "` + secretToken + `",
				"createdBy": {
					"id": 9,
					"displayName": "Manager",
					"type": "user"
				},
				"properties": {
					"region": "North"
				}
			},
			{
				"id": 116,
				"projectId": 7,
				"displayName": "Revoked Collector",
				"type": "field_key"
			}
		]`))
	}))
	defer server.Close()

	client := &CentralClient{
		BaseURL:    server.URL,
		Token:      "central-session-token",
		HTTPClient: server.Client(),
	}

	appUsers, err := listProjectAppUsers(client, 7)
	if err != nil {
		t.Fatalf("listProjectAppUsers returned error: %v", err)
	}
	if len(appUsers) != 2 {
		t.Fatalf("expected 2 App Users, got %d", len(appUsers))
	}

	active := appUsers[0]
	if active.ID != 115 || active.ProjectID != 7 || active.DisplayName != "Collector North" {
		t.Fatalf("unexpected active App User: %+v", active)
	}
	if active.Revoked {
		t.Fatalf("expected App User with a token to be active")
	}
	if active.LastUsed == nil || !active.LastUsed.Equal(time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected last-used timestamp: %v", active.LastUsed)
	}
	if active.CreatedBy == nil || active.CreatedBy.ID != 9 {
		t.Fatalf("unexpected creator: %+v", active.CreatedBy)
	}
	if active.Properties["region"] != "North" {
		t.Fatalf("unexpected properties: %+v", active.Properties)
	}

	if !appUsers[1].Revoked {
		t.Fatalf("expected App User without a token to be revoked")
	}

	encoded, err := json.Marshal(appUsers)
	if err != nil {
		t.Fatalf("failed to marshal decoded App Users: %v", err)
	}
	if strings.Contains(string(encoded), secretToken) || strings.Contains(string(encoded), `"token"`) {
		t.Fatalf("decoded App Users retained token data: %s", encoded)
	}
}

func TestListProjectAppUsersDoesNotExposeTokenOnDecodeError(t *testing.T) {
	const secretToken = "secret-app-user-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"invalid","token":"` + secretToken + `"}]`))
	}))
	defer server.Close()

	client := &CentralClient{
		BaseURL:    server.URL,
		Token:      "central-session-token",
		HTTPClient: server.Client(),
	}

	_, err := listProjectAppUsers(client, 7)
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Fatalf("decode error exposed token: %v", err)
	}
}

func TestListProjectAppUsersDoesNotExposeNonOKResponseBody(t *testing.T) {
	const secretToken = "secret-app-user-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"token":"`+secretToken+`"}`, http.StatusForbidden)
	}))
	defer server.Close()

	client := &CentralClient{
		BaseURL:    server.URL,
		Token:      "central-session-token",
		HTTPClient: server.Client(),
	}

	_, err := listProjectAppUsers(client, 7)
	if err == nil {
		t.Fatalf("expected non-OK response error")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Fatalf("non-OK response error exposed token: %v", err)
	}
}
