package main

import (
	"fmt"
	"time"
)

func approveSubmission(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	instanceID string,
) error {
	requestURL := fmt.Sprintf(
		"%s/v1/projects/%d/forms/%s/submissions/%s",
		client.BaseURL,
		projectID,
		xmlFormID,
		instanceID,
	)

	payload := map[string]interface{}{
		"reviewState": "approved",
	}

	if err := client.DoJSON("PATCH", requestURL, payload, nil); err != nil {
		return fmt.Errorf("failed to approve submission %s: %w", instanceID, err)
	}

	return nil
}

func addSubmissionSyncComment(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	instanceID string,
) error {
	requestURL := fmt.Sprintf(
		"%s/v1/projects/%d/forms/%s/submissions/%s/comments",
		client.BaseURL,
		projectID,
		xmlFormID,
		instanceID,
	)

	payload := map[string]interface{}{
		"body": "Synced at " + time.Now().In(time.Local).Format("2006-01-02 15:04:05 MST"),
	}

	if err := client.DoJSON("POST", requestURL, payload, nil); err != nil {
		return fmt.Errorf("failed to post sync comment for submission %s: %w", instanceID, err)
	}

	return nil
}