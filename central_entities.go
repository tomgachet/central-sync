package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ODataEntitiesResponse struct {
	Context  string                   `json:"@odata.context"`
	Count    *int                     `json:"@odata.count,omitempty"`
	NextLink string                   `json:"@odata.nextLink,omitempty"`
	Value    []map[string]interface{} `json:"value"`
}

func getDatasetEntities(client *CentralClient, projectID int, datasetName string) (*ODataEntitiesResponse, error) {
	url := fmt.Sprintf(
		"%s/v1/projects/%d/datasets/%s.svc/Entities?$top=1000&$count=true",
		client.BaseURL,
		projectID,
		datasetName,
	)

	return fetchDatasetEntitiesPage(client, url)
}

func getAllDatasetEntities(client *CentralClient, projectID int, datasetName string) ([]map[string]interface{}, error) {
	firstPage, err := getDatasetEntities(client, projectID, datasetName)
	if err != nil {
		return nil, err
	}

	allEntities := make([]map[string]interface{}, 0, len(firstPage.Value))
	allEntities = append(allEntities, firstPage.Value...)

	nextLink := firstPage.NextLink

	for nextLink != "" {
		page, err := fetchDatasetEntitiesPage(client, nextLink)
		if err != nil {
			return nil, err
		}

		allEntities = append(allEntities, page.Value...)
		nextLink = page.NextLink
	}

	return allEntities, nil
}

func fetchDatasetEntitiesPage(client *CentralClient, url string) (*ODataEntitiesResponse, error) {
	resp, err := client.Get(url)
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