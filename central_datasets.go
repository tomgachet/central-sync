package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func getDatasetMetadata(client *CentralClient, projectID int, datasetName string) (*DatasetMetadata, error) {
	url := fmt.Sprintf("%s/v1/projects/%d/datasets/%s", client.BaseURL, projectID, datasetName)

	resp, err := client.Get(url)
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