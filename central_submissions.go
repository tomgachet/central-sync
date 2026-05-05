package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ODataSubmissionsResponse struct {
	Context  string                   `json:"@odata.context"`
	Count    *int                     `json:"@odata.count,omitempty"`
	NextLink string                   `json:"@odata.nextLink,omitempty"`
	Value    []map[string]interface{} `json:"value"`
}

func getFormSubmissions(client *CentralClient, projectID int, xmlFormID string) (*ODataSubmissionsResponse, error) {
	url := fmt.Sprintf(
		"%s/v1/projects/%d/forms/%s.svc/Submissions?$top=1000&$count=true",
		client.BaseURL,
		projectID,
		xmlFormID,
	)

	return fetchFormSubmissionsPage(client, url)
}

func getAllFormSubmissions(client *CentralClient, projectID int, xmlFormID string) ([]map[string]interface{}, error) {
	firstPage, err := getFormSubmissions(client, projectID, xmlFormID)
	if err != nil {
		return nil, err
	}

	allRows := make([]map[string]interface{}, 0, len(firstPage.Value))
	allRows = append(allRows, firstPage.Value...)

	nextLink := firstPage.NextLink

	for nextLink != "" {
		page, err := fetchFormSubmissionsPage(client, nextLink)
		if err != nil {
			return nil, err
		}

		allRows = append(allRows, page.Value...)
		nextLink = page.NextLink
	}

	return allRows, nil
}

func fetchFormSubmissionsPage(client *CentralClient, url string) (*ODataSubmissionsResponse, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call form submissions endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read form submissions response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from form submissions endpoint: %s - %s", resp.Status, string(body))
	}

	var result ODataSubmissionsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode form submissions response: %w", err)
	}

	return &result, nil
}