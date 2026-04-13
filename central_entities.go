package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ODataEntitiesResponse struct {
	Context  string                   `json:"@odata.context"`
	Count    *int                     `json:"@odata.count,omitempty"`
	NextLink string                   `json:"@odata.nextLink,omitempty"`
	Value    []map[string]interface{} `json:"value"`
}

func getDatasetEntities(centralURL, token string, projectID int, datasetName string) (*ODataEntitiesResponse, error) {
	url := fmt.Sprintf(
		"%s/v1/projects/%d/datasets/%s.svc/Entities?$top=1000&$count=true",
		centralURL,
		projectID,
		datasetName,
	)

	return fetchDatasetEntitiesPage(url, token)
}

func getAllDatasetEntities(centralURL, token string, projectID int, datasetName string) ([]map[string]interface{}, error) {
	firstPage, err := getDatasetEntities(centralURL, token, projectID, datasetName)
	if err != nil {
		return nil, err
	}

	allEntities := make([]map[string]interface{}, 0, len(firstPage.Value))
	allEntities = append(allEntities, firstPage.Value...)

	nextLink := firstPage.NextLink

	for nextLink != "" {
		page, err := fetchDatasetEntitiesPage(nextLink, token)
		if err != nil {
			return nil, err
		}

		allEntities = append(allEntities, page.Value...)
		nextLink = page.NextLink
	}

	return allEntities, nil
}

func fetchDatasetEntitiesPage(url, token string) (*ODataEntitiesResponse, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset entities request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call dataset entities endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset entities response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from dataset entities endpoint: %s - %s", resp.Status, string(body))
	}

	var result ODataEntitiesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode dataset entities response: %w", err)
	}

	return &result, nil
}