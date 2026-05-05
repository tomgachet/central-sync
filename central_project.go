package main

import (
	"fmt"
	"io"
	"net/http"
)

func projectExists(client *CentralClient, projectID int) (bool, error) {
	url := fmt.Sprintf("%s/v1/projects/%d", client.BaseURL, projectID)

	resp, err := client.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to call project endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	body, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("unexpected response from project endpoint: %s - %s", resp.Status, string(body))
}