package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type DatasetMapping struct {
	Name      string `yaml:"name"`
	TableName string `yaml:"table_name"`
	Sync      bool   `yaml:"sync"`
}

type ProjectMapping struct {
	ProjectID    int    `yaml:"project_id"`
	ProjectName  string `yaml:"project_name"`
	DatabaseName string `yaml:"database_name"`
	Datasets     []DatasetMapping `yaml:"datasets"`
}

type ProjectConfig struct {
	Projects []ProjectMapping `yaml:"projects"`
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line in %s at line %d", path, lineNumber)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	return nil
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

func getRequiredEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("missing %s", key)
	}

	return value, nil
}