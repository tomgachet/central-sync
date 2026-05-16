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
	return c.GetWithAccept(url, "application/json")
}

func (c *CentralClient) GetWithAccept(url string, accept string) (*http.Response, error) {
	resp, err := c.doGetWithAccept(url, c.Token, accept)
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

	return c.doGetWithAccept(url, c.Token, accept)
}

func (c *CentralClient) doGetWithAccept(url string, token string, accept string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)

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