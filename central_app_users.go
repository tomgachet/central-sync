package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type CentralActor struct {
	ID          int64      `json:"id"`
	DisplayName string     `json:"displayName"`
	Type        string     `json:"type"`
	CreatedAt   *time.Time `json:"createdAt"`
	UpdatedAt   *time.Time `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt"`
}

type CentralAppUser struct {
	ID          int64             `json:"id"`
	ProjectID   int               `json:"projectId"`
	DisplayName string            `json:"displayName"`
	Type        string            `json:"type"`
	CreatedAt   *time.Time        `json:"createdAt"`
	UpdatedAt   *time.Time        `json:"updatedAt"`
	DeletedAt   *time.Time        `json:"deletedAt"`
	LastUsed    *time.Time        `json:"lastUsed"`
	CreatedBy   *CentralActor     `json:"createdBy"`
	Properties  map[string]string `json:"properties"`
	Revoked     bool              `json:"revoked"`
}

func (u *CentralAppUser) UnmarshalJSON(data []byte) error {
	type appUserWithoutToken CentralAppUser
	var wire struct {
		appUserWithoutToken
		Token json.RawMessage `json:"token"`
	}

	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	*u = CentralAppUser(wire.appUserWithoutToken)
	u.Revoked = len(wire.Token) == 0 || bytes.Equal(bytes.TrimSpace(wire.Token), []byte("null"))
	return nil
}

func listProjectAppUsers(client *CentralClient, projectID int) ([]CentralAppUser, error) {
	requestURL := fmt.Sprintf("%s/v1/projects/%d/app-users", client.BaseURL, projectID)

	resp, err := client.GetWithExtendedMetadata(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to call App Users endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read App Users response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("App Users endpoint returned %s", resp.Status)
	}

	var appUsers []CentralAppUser
	if err := json.Unmarshal(body, &appUsers); err != nil {
		return nil, fmt.Errorf("failed to decode App Users response")
	}

	return appUsers, nil
}
