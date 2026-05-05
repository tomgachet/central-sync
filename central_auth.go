package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const centralSessionFile = "central_session.json"

type sessionRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

type SavedCentralSession struct {
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

func loadSavedCentralSession(path string) (*SavedCentralSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session SavedCentralSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &session, nil
}

func saveCentralSession(path string, token string, expiresAt string) error {
	session := SavedCentralSession{
		Token:     token,
		ExpiresAt: expiresAt,
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode session file: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

func isCentralSessionExpired(session *SavedCentralSession) (bool, error) {
	if session == nil || session.Token == "" || session.ExpiresAt == "" {
		return true, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil {
		return true, fmt.Errorf("failed to parse session expiration: %w", err)
	}

	return !time.Now().Before(expiresAt), nil
}

func getValidCentralToken() (string, error) {
	savedSession, err := loadSavedCentralSession(centralSessionFile)
	if err != nil {
		return "", err
	}

	expired, err := isCentralSessionExpired(savedSession)
	if err != nil {
		return "", err
	}

	if savedSession != nil && !expired {
		return savedSession.Token, nil
	}

	session, err := initCentralSession()
	if err != nil {
		return "", err
	}

	if err := saveCentralSession(centralSessionFile, session.Token, session.ExpiresAt); err != nil {
		return "", err
	}

	return session.Token, nil
}