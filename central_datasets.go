package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type DatasetProperty struct {
	Name      string `json:"name"`
	ODataName string `json:"odataName"`
}

type DatasetMetadata struct {
	Name       string            `json:"name"`
	ProjectID  int               `json:"projectId"`
	Properties []DatasetProperty `json:"properties"`
}

func getDatasetMetadata(centralURL string, token string, projectID int, datasetName string) (*DatasetMetadata, error) {
	url := fmt.Sprintf("%s/v1/projects/%d/datasets/%s", centralURL, projectID, datasetName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset metadata request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call dataset metadata endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset metadata response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from dataset metadata endpoint: %s - %s", resp.Status, string(body))
	}

	var metadata DatasetMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, fmt.Errorf("failed to decode dataset metadata response: %w", err)
	}

	return &metadata, nil
}