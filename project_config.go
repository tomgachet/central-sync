package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type DatasetMapping struct {
	Name      string `yaml:"name"`
	TableName string `yaml:"table_name"`
	Sync      bool   `yaml:"sync"`
}

type ProjectMapping struct {
	ProjectID    int              `yaml:"project_id"`
	ProjectName  string           `yaml:"project_name"`
	DatabaseName string           `yaml:"database_name"`
	Datasets     []DatasetMapping `yaml:"datasets"`
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