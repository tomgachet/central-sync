package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "central_config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return path
}

func TestLoadProjectConfigAcceptsValidConfig(t *testing.T) {
	path := writeProjectConfigFile(t, `projects:
  - project_id: 1
    project_name: Demo
    database_name: demo_db
    datasets:
      - name: people
        table_name: central_people
        sync: true
    forms:
      - xml_form_id: household_form
        table_name: household_submissions
        sync: true
        sync_mode: upsert
`)

	config, err := loadProjectConfig(path)
	if err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}
	if len(config.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(config.Projects))
	}
}

func TestLoadProjectConfigDefaultsEmptyFormSyncMode(t *testing.T) {
	path := writeProjectConfigFile(t, `projects:
  - project_id: 1
    database_name: demo_db
    forms:
      - xml_form_id: household_form
        table_name: household_submissions
        sync: true
`)

	config, err := loadProjectConfig(path)
	if err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}

	if got := getFormSyncMode(config.Projects[0].Forms[0]); got != SyncModeAppendOnly {
		t.Fatalf("expected default sync mode %q, got %q", SyncModeAppendOnly, got)
	}
}

func TestLoadProjectConfigRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectedErr string
	}{
		{
			name: "invalid project id",
			config: `projects:
  - project_id: 0
    database_name: demo_db
`,
			expectedErr: "projects[0].project_id must be greater than 0",
		},
		{
			name: "missing database name",
			config: `projects:
  - project_id: 1
    database_name: ""
`,
			expectedErr: "projects[0].database_name is required",
		},
		{
			name: "missing dataset name",
			config: `projects:
  - project_id: 1
    database_name: demo_db
    datasets:
      - table_name: central_people
`,
			expectedErr: "projects[0].datasets[0].name is required",
		},
		{
			name: "missing dataset table",
			config: `projects:
  - project_id: 1
    database_name: demo_db
    datasets:
      - name: people
        table_name: ""
`,
			expectedErr: "projects[0].datasets[0].table_name is required",
		},
		{
			name: "missing form id",
			config: `projects:
  - project_id: 1
    database_name: demo_db
    forms:
      - table_name: household_submissions
`,
			expectedErr: "projects[0].forms[0].xml_form_id is required",
		},
		{
			name: "missing form table",
			config: `projects:
  - project_id: 1
    database_name: demo_db
    forms:
      - xml_form_id: household_form
        table_name: ""
`,
			expectedErr: "projects[0].forms[0].table_name is required",
		},
		{
			name: "unknown sync mode",
			config: `projects:
  - project_id: 1
    database_name: demo_db
    forms:
      - xml_form_id: household_form
        table_name: household_submissions
        sync_mode: replace
`,
			expectedErr: "projects[0].forms[0].sync_mode must be empty",
		},
		{
			name: "duplicate table name",
			config: `projects:
  - project_id: 1
    database_name: demo_db
    datasets:
      - name: people
        table_name: central_records
    forms:
      - xml_form_id: household_form
        table_name: central_records
`,
			expectedErr: `projects[0].forms[0].table_name "central_records" duplicates projects[0].datasets[0].table_name`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeProjectConfigFile(t, tt.config)

			_, err := loadProjectConfig(path)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("expected error containing %q, got %v", tt.expectedErr, err)
			}
		})
	}
}
