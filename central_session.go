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

type sessionRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Token   string `json:"token"`
	Expires string `json:"expiresAt"`
}

func initCentralSession(envPath string) (string, error) {
	err := loadEnvFile(envPath)
	if err != nil {
		return "", fmt.Errorf("loading error .env: %w", err)
	}

	centralURL := os.Getenv("ODK_CENTRAL_URL")
	email := os.Getenv("ODK_CENTRAL_USER_EMAIL")
	password := os.Getenv("ODK_CENTRAL_USER_PASSWORD")

	if centralURL == "" {
		return "", fmt.Errorf("ODK_CENTRAL_URL manquant")
	}
	if email == "" {
		return "", fmt.Errorf("ODK_CENTRAL_USER_EMAIL manquant")
	}
	if password == "" {
		return "", fmt.Errorf("ODK_CENTRAL_USER_PASSWORD manquant")
	}

	token, err := createCentralSession(centralURL, email, password)
	if err != nil {
		return "", err
	}

	return token, nil
}

func createCentralSession(centralURL, email, password string) (string, error) {
	payload := sessionRequest{
		Email:    email,
		Password: password,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("JSON encoding error: %w", err)
	}

	url := centralURL + "/v1/sessions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("session request creation error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("session call error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error read response session: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("authentication failed: %s - %s", resp.Status, string(respBody))
	}

	var session sessionResponse
	if err := json.Unmarshal(respBody, &session); err != nil {
		return "", fmt.Errorf("session decoding error: %w", err)
	}

	if session.Token == "" {
		return "", fmt.Errorf("empty token in the response")
	}

	return session.Token, nil
}