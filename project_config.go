package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	SyncModeAppendOnly = "append_only"
	SyncModeUpsert     = "upsert"
)

type DatasetMapping struct {
	Name      string `yaml:"name"`
	TableName string `yaml:"table_name"`
	Sync      bool   `yaml:"sync"`
}

type FormMapping struct {
	XMLFormID string `yaml:"xml_form_id"`
	TableName string `yaml:"table_name"`
	Sync      bool   `yaml:"sync"`
	SyncMode  string `yaml:"sync_mode"`
	ApprovedOnly bool   `yaml:"approved_only"`
	ApproveAfterSync bool   `yaml:"approve_after_sync"`
}

type ProjectMapping struct {
	ProjectID    int              `yaml:"project_id"`
	ProjectName  string           `yaml:"project_name"`
	DatabaseName string           `yaml:"database_name"`
	Datasets     []DatasetMapping `yaml:"datasets"`
	Forms        []FormMapping    `yaml:"forms"`
}

type ProjectConfig struct {
	Projects []ProjectMapping `yaml:"projects"`
}

func loadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config file: %w", err)
	}

	var config ProjectConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config file: %w", err)
	}

	return &config, nil
}

func getDatasetsToSync(project ProjectMapping) []DatasetMapping {
	var datasetsToSync []DatasetMapping

	for _, dataset := range project.Datasets {
		if dataset.Sync {
			datasetsToSync = append(datasetsToSync, dataset)
		}
	}

	return datasetsToSync
}

func getFormsToSync(project ProjectMapping) []FormMapping {
	var formsToSync []FormMapping

	for _, form := range project.Forms {
		if form.Sync {
			formsToSync = append(formsToSync, form)
		}
	}

	return formsToSync
}

func getFormSyncMode(form FormMapping) string {
	switch form.SyncMode {
	case SyncModeAppendOnly, SyncModeUpsert:
		return form.SyncMode
	default:
		return SyncModeAppendOnly
	}
}