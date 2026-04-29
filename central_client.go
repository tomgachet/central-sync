package main

import (
	"fmt"
	"net/http"
	"time"
)

type CentralClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func newCentralClient() (*CentralClient, error) {
	baseURL, err := getRequiredEnv("ODK_CENTRAL_URL")
	if err != nil {
		return nil, err
	}

	token, err := getValidCentralToken()
	if err != nil {
		return nil, err
	}

	return &CentralClient{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *CentralClient) Get(url string) (*http.Response, error) {
	resp, err := c.doGet(url, c.Token)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	resp.Body.Close()

	err = c.refreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh Central token after 401: %w", err)
	}

	return c.doGet(url, c.Token)
}

func (c *CentralClient) doGet(url string, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GET request: %w", err)
	}

	return resp, nil
}

func (c *CentralClient) refreshToken() error {
	session, err := initCentralSession()
	if err != nil {
		return err
	}

	err = saveCentralSession(centralSessionFile, session.Token, session.ExpiresAt)
	if err != nil {
		return err
	}

	c.Token = session.Token
	return nil
}