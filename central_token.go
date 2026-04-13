package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const centralSessionFile = "central_session.json"

type SavedCentralSession struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
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