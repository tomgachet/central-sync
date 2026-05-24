package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type ODataTableRowsResponse struct {
	Context  string                   `json:"@odata.context"`
	Count    *int                     `json:"@odata.count,omitempty"`
	NextLink string                   `json:"@odata.nextLink,omitempty"`
	Value    []map[string]interface{} `json:"value"`
}

func getFormTableRows(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	odataTableURL string,
	filter string,
) (*ODataTableRowsResponse, error) {
	base := fmt.Sprintf(
		"%s/v1/projects/%d/forms/%s.svc/%s",
		client.BaseURL,
		projectID,
		xmlFormID,
		odataTableURL,
	)

	params := url.Values{}
	params.Set("$top", "1000")
	params.Set("$count", "true")

	if filter != "" {
		params.Set("$filter", filter)
	}

	return fetchFormTableRowsPage(client, base+"?"+params.Encode())
}

func getAllFormTableRows(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	odataTableURL string,
	filter string,
) ([]map[string]interface{}, error) {
	firstPage, err := getFormTableRows(client, projectID, xmlFormID, odataTableURL, filter)
	if err != nil {
		return nil, err
	}

	allRows := make([]map[string]interface{}, 0, len(firstPage.Value))
	allRows = append(allRows, firstPage.Value...)

	nextLink := firstPage.NextLink

	for nextLink != "" {
		page, err := fetchFormTableRowsPage(client, nextLink)
		if err != nil {
			return nil, err
		}

		allRows = append(allRows, page.Value...)
		nextLink = page.NextLink
	}

	return allRows, nil
}

func fetchFormTableRowsPage(client *CentralClient, requestURL string) (*ODataTableRowsResponse, error) {
	resp, err := client.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to call form table endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read form table response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from form table endpoint: %s - %s", resp.Status, string(body))
	}

	var result ODataTableRowsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode form table response: %w", err)
	}

	return &result, nil
}

func getSubmissionRowsByUUIDs(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	submissionUUIDs []string,
) ([]map[string]interface{}, error) {
	if len(submissionUUIDs) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var allRows []map[string]interface{}

	for _, submissionUUID := range submissionUUIDs {
		if submissionUUID == "" || seen[submissionUUID] {
			continue
		}
		seen[submissionUUID] = true

		filter := fmt.Sprintf("__id eq 'uuid:%s'", submissionUUID)

		rows, err := getAllFormTableRows(
			client,
			projectID,
			xmlFormID,
			"Submissions",
			filter,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch submission %s: %w", submissionUUID, err)
		}

		allRows = append(allRows, rows...)
	}

	return allRows, nil
}