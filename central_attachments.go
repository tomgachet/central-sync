package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type SubmissionAttachment struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

func listSubmissionAttachments(
	client *CentralClient,
	projectID int,
	formXMLID string,
	instanceID string,
) ([]SubmissionAttachment, error) {
	requestURL := fmt.Sprintf(
		"%s/v1/projects/%d/forms/%s/submissions/%s/attachments",
		client.BaseURL,
		projectID,
		url.PathEscape(formXMLID),
		url.PathEscape(instanceID),
	)

	resp, err := client.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list submission attachments: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read submission attachments response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("non-OK response from submission attachments endpoint: %s - %s", resp.Status, string(body))
	}

	var attachments []SubmissionAttachment
	if err := json.Unmarshal(body, &attachments); err != nil {
		return nil, fmt.Errorf("failed to decode submission attachments response: %w", err)
	}

	return attachments, nil
}

func downloadSubmissionAttachment(
	client *CentralClient,
	projectID int,
	formXMLID string,
	instanceID string,
	filename string,
) (*http.Response, error) {
	requestURL := fmt.Sprintf(
		"%s/v1/projects/%d/forms/%s/submissions/%s/attachments/%s",
		client.BaseURL,
		projectID,
		url.PathEscape(formXMLID),
		url.PathEscape(instanceID),
		url.PathEscape(filename),
	)

	resp, err := client.GetAttachment(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download submission attachment %q: %w", filename, err)
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return resp, nil
	}

	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("attachment download returned %s and its response body could not be read: %w", resp.Status, readErr)
	}

	return nil, fmt.Errorf("non-OK response while downloading submission attachment %q: %s - %s", filename, resp.Status, string(body))
}
