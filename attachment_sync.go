package main

import (
	"fmt"
	"strings"
)

type SyncedAttachment struct {
	Name           string
	RelativePath   string
	ContentType    string
	ETag           string
	SizeBytes      int64
	ChecksumSHA256 string
	Replaced       bool
	Unchanged      bool
}

type AttachmentSyncResult struct {
	Expected int
	Present  int
	Stored   []SyncedAttachment
	Missing  []string
}

func syncSubmissionAttachments(
	client *CentralClient,
	storage AttachmentStorage,
	projectID int,
	formXMLID string,
	submissionUUID string,
) (*AttachmentSyncResult, error) {
	if client == nil {
		return nil, fmt.Errorf("Central client is required for attachment sync")
	}
	if storage == nil {
		return nil, fmt.Errorf("attachment storage is required")
	}

	instanceID := centralSubmissionInstanceID(submissionUUID)
	attachments, err := listSubmissionAttachments(client, projectID, formXMLID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to sync attachments for submission %s: %w", submissionUUID, err)
	}

	result := &AttachmentSyncResult{Expected: len(attachments)}

	for _, attachment := range attachments {
		if !attachment.Exists {
			result.Missing = append(result.Missing, attachment.Name)
			continue
		}

		result.Present++
		stored, err := syncSingleSubmissionAttachment(
			client,
			storage,
			projectID,
			formXMLID,
			instanceID,
			submissionUUID,
			attachment.Name,
		)
		if err != nil {
			return result, err
		}
		result.Stored = append(result.Stored, *stored)
	}

	return result, nil
}

func syncSingleSubmissionAttachment(
	client *CentralClient,
	storage AttachmentStorage,
	projectID int,
	formXMLID string,
	instanceID string,
	submissionUUID string,
	filename string,
) (*SyncedAttachment, error) {
	response, err := downloadSubmissionAttachment(client, projectID, formXMLID, instanceID, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to sync attachment %q for submission %s: %w", filename, submissionUUID, err)
	}
	defer response.Body.Close()

	stored, err := storage.Store(AttachmentStorageKey{
		ProjectID:      projectID,
		FormXMLID:      formXMLID,
		SubmissionUUID: submissionUUID,
		Filename:       filename,
	}, response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to store attachment %q for submission %s: %w", filename, submissionUUID, err)
	}

	return &SyncedAttachment{
		Name:           filename,
		RelativePath:   stored.RelativePath,
		ContentType:    response.Header.Get("Content-Type"),
		ETag:           response.Header.Get("ETag"),
		SizeBytes:      stored.SizeBytes,
		ChecksumSHA256: stored.ChecksumSHA256,
		Replaced:       stored.Replaced,
		Unchanged:      stored.Unchanged,
	}, nil
}

func centralSubmissionInstanceID(submissionUUID string) string {
	if strings.HasPrefix(submissionUUID, "uuid:") {
		return submissionUUID
	}
	return "uuid:" + submissionUUID
}
