package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildDatasetFilter(t *testing.T) {
	created := timePtr(t, "2026-11-11T15:48:18.248Z")
	updated := timePtr(t, "2026-11-11T16:00:00Z")
	for _, tt := range []struct {
		name string
		last *LastSuccessfulDatasetSync
		want string
	}{
		{"initial load", nil, ""},
		{"no watermarks", &LastSuccessfulDatasetSync{}, ""},
		{"creation only includes first updates", &LastSuccessfulDatasetSync{MaxCreatedAt: created}, "(__system/createdAt gt 2026-11-11T15:48:18.248Z) or (__system/updatedAt ne null)"},
		{"update only still includes new entities", &LastSuccessfulDatasetSync{MaxUpdatedAt: updated}, ""},
		{"both watermarks", &LastSuccessfulDatasetSync{MaxCreatedAt: created, MaxUpdatedAt: updated}, "(__system/createdAt gt 2026-11-11T15:48:18.248Z) or (__system/updatedAt gt 2026-11-11T16:00:00.000Z)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildDatasetFilter(tt.last); got != tt.want {
				t.Fatalf("filter = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDeletedDatasetFilter(t *testing.T) {
	deleted := timePtr(t, "2026-11-11T16:00:00Z")
	for _, last := range []*LastSuccessfulDatasetSync{nil, {}, {MaxCreatedAt: deleted}} {
		if got := buildDeletedDatasetFilter(last); got != "__system/deletedAt ne null" {
			t.Fatalf("unexpected initial deletion filter: %s", got)
		}
	}
	if got := buildDeletedDatasetFilter(&LastSuccessfulDatasetSync{MaxDeletedAt: deleted}); got != "(__system/deletedAt ne null) and (__system/deletedAt gt 2026-11-11T16:00:00.000Z)" {
		t.Fatalf("unexpected incremental deletion filter: %s", got)
	}
}

// The server checks the actual decoded OData query, including the transition
// from no update watermark to a timestamp. It is a fixture, not a Central server.
func TestDatasetIncrementalRequests(t *testing.T) {
	created := timePtr(t, "2026-11-11T15:48:18.248Z")
	updated := timePtr(t, "2026-11-11T16:00:00Z")
	bootstrap := "(__system/createdAt gt 2026-11-11T15:48:18.248Z) or (__system/updatedAt ne null)"
	incremental := "(__system/createdAt gt 2026-11-11T15:48:18.248Z) or (__system/updatedAt gt 2026-11-11T16:00:00.000Z)"
	steps := []struct {
		name    string
		last    *LastSuccessfulDatasetSync
		filter  string
		version int
	}{
		{"initial load", nil, "", 1},
		{"empty run", &LastSuccessfulDatasetSync{MaxCreatedAt: created}, bootstrap, 0},
		{"first update of older entity after empty run", &LastSuccessfulDatasetSync{MaxCreatedAt: created}, bootstrap, 2},
		{"later incremental update", &LastSuccessfulDatasetSync{MaxCreatedAt: created, MaxUpdatedAt: updated}, incremental, 3},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/projects/1/datasets/etudes.svc/Entities" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if got := r.URL.Query().Get("$filter"); got != step.filter {
					t.Errorf("filter = %q, want %q", got, step.filter)
				}
				rows := []map[string]interface{}{}
				if step.version > 0 {
					rows = append(rows, map[string]interface{}{"__id": "old-entity", "__system": map[string]interface{}{"createdAt": "2026-01-01T00:00:00Z", "version": step.version}})
				}
				_ = json.NewEncoder(w).Encode(ODataEntitiesResponse{Value: rows})
			}))
			defer server.Close()
			client := &CentralClient{BaseURL: server.URL, HTTPClient: server.Client()}
			entities, err := getAllDatasetEntities(client, 1, "etudes", buildDatasetFilter(step.last))
			if err != nil {
				t.Fatal(err)
			}
			if step.version == 0 {
				if len(entities) != 0 {
					t.Fatal("expected empty run")
				}
				return
			}
			if len(entities) != 1 {
				t.Fatalf("expected one entity, got %d", len(entities))
			}
			system, err := getEntitySystemData(entities[0])
			if err != nil {
				t.Fatal(err)
			}
			if system.Version != step.version {
				t.Fatalf("version = %d, want %d", system.Version, step.version)
			}
		})
	}
}
