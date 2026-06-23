package main

import (
	"fmt"
	"os"
	"strings"

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
	XMLFormID        string `yaml:"xml_form_id"`
	TableName        string `yaml:"table_name"`
	Sync             bool   `yaml:"sync"`
	SyncMode         string `yaml:"sync_mode"`
	ApprovedOnly     bool   `yaml:"approved_only"`
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

	if err := validateProjectConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid YAML config file: %w", err)
	}

	return &config, nil
}

func validateProjectConfig(config *ProjectConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	for projectIndex, project := range config.Projects {
		projectRef := fmt.Sprintf("projects[%d]", projectIndex)

		if project.ProjectID <= 0 {
			return fmt.Errorf("%s.project_id must be greater than 0", projectRef)
		}
		if strings.TrimSpace(project.DatabaseName) == "" {
			return fmt.Errorf("%s.database_name is required", projectRef)
		}

		tableNames := make(map[string]string)
		for datasetIndex, dataset := range project.Datasets {
			datasetRef := fmt.Sprintf("%s.datasets[%d]", projectRef, datasetIndex)

			if strings.TrimSpace(dataset.Name) == "" {
				return fmt.Errorf("%s.name is required", datasetRef)
			}
			if err := validateMappedTableName(tableNames, strings.TrimSpace(dataset.TableName), datasetRef); err != nil {
				return err
			}
		}

		for formIndex, form := range project.Forms {
			formRef := fmt.Sprintf("%s.forms[%d]", projectRef, formIndex)

			if strings.TrimSpace(form.XMLFormID) == "" {
				return fmt.Errorf("%s.xml_form_id is required", formRef)
			}
			if err := validateMappedTableName(tableNames, strings.TrimSpace(form.TableName), formRef); err != nil {
				return err
			}
			if err := validateFormSyncMode(strings.TrimSpace(form.SyncMode), formRef); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateMappedTableName(tableNames map[string]string, tableName string, mappingRef string) error {
	if tableName == "" {
		return fmt.Errorf("%s.table_name is required", mappingRef)
	}

	if previousRef, exists := tableNames[tableName]; exists {
		return fmt.Errorf("%s.table_name %q duplicates %s.table_name", mappingRef, tableName, previousRef)
	}
	tableNames[tableName] = mappingRef

	return nil
}

func validateFormSyncMode(syncMode string, formRef string) error {
	switch syncMode {
	case "", SyncModeAppendOnly, SyncModeUpsert:
		return nil
	default:
		return fmt.Errorf("%s.sync_mode must be empty, %q, or %q", formRef, SyncModeAppendOnly, SyncModeUpsert)
	}
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
