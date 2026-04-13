package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type sessionRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

func initCentralSession() (*sessionResponse, error) {
	centralURL, err := getRequiredEnv("ODK_CENTRAL_URL")
	if err != nil {
		return nil, err
	}

	email, err := getRequiredEnv("ODK_CENTRAL_USER_EMAIL")
	if err != nil {
		return nil, err
	}

	password, err := getRequiredEnv("ODK_CENTRAL_USER_PASSWORD")
	if err != nil {
		return nil, err
	}

	return createCentralSession(centralURL, email, password)
}

func createCentralSession(centralURL, email, password string) (*sessionResponse, error) {
	payload := sessionRequest{
		Email:    email,
		Password: password,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode session payload: %w", err)
	}

	url := centralURL + "/v1/sessions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create session request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call session endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read session response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("authentication failed: %s - %s", resp.Status, string(respBody))
	}

	var session sessionResponse
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, fmt.Errorf("failed to decode session response: %w", err)
	}

	if session.Token == "" {
		return nil, fmt.Errorf("empty token in session response")
	}
	if session.ExpiresAt == "" {
		return nil, fmt.Errorf("empty expiresAt in session response")
	}

	return &session, nil
}