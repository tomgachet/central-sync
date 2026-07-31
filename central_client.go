package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type CentralClient struct {
	BaseURL              string
	Token                string
	HTTPClient           *http.Client
	AttachmentHTTPClient *http.Client
}

const (
	centralRequestTimeout    = 30 * time.Second
	attachmentRequestTimeout = 10 * time.Minute
)

type centralHTTPResponse struct {
	StatusCode int
	Status     string
	Body       []byte
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

	httpClient := &http.Client{
		Timeout: centralRequestTimeout,
	}
	attachmentHTTPClient := *httpClient
	attachmentHTTPClient.Timeout = attachmentRequestTimeout

	return &CentralClient{
		BaseURL:              baseURL,
		Token:                token,
		HTTPClient:           httpClient,
		AttachmentHTTPClient: &attachmentHTTPClient,
	}, nil
}

func (c *CentralClient) Get(url string) (*http.Response, error) {
	return c.GetWithAccept(url, "application/json")
}

func (c *CentralClient) GetWithAccept(url string, accept string) (*http.Response, error) {
	return c.getWithAcceptUsingClient(url, accept, false, c.HTTPClient)
}

func (c *CentralClient) GetWithExtendedMetadata(url string) (*http.Response, error) {
	return c.getWithAcceptUsingClient(url, "application/json", true, c.HTTPClient)
}

func (c *CentralClient) GetAttachment(url string) (*http.Response, error) {
	httpClient := c.AttachmentHTTPClient
	if httpClient == nil {
		httpClient = c.HTTPClient
	}
	return c.getWithAcceptUsingClient(url, "*/*", false, httpClient)
}

func (c *CentralClient) getWithAcceptUsingClient(
	url string,
	accept string,
	extendedMetadata bool,
	httpClient *http.Client,
) (*http.Response, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("Central HTTP client is not configured")
	}

	resp, err := c.doGetWithAccept(url, c.Token, accept, extendedMetadata, httpClient)
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

	return c.doGetWithAccept(url, c.Token, accept, extendedMetadata, httpClient)
}

func (c *CentralClient) doGetWithAccept(
	url string,
	token string,
	accept string,
	extendedMetadata bool,
	httpClient *http.Client,
) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	if extendedMetadata {
		req.Header.Set("X-Extended-Metadata", "true")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GET request: %w", err)
	}

	return resp, nil
}

func (c *CentralClient) DoJSON(method string, requestURL string, body interface{}, out interface{}) error {
	result, err := c.doJSON(method, requestURL, c.Token, body)
	if err != nil {
		return err
	}

	if result.StatusCode == http.StatusUnauthorized {
		if err := c.refreshToken(); err != nil {
			return fmt.Errorf("failed to refresh Central token after 401: %w", err)
		}

		result, err = c.doJSON(method, requestURL, c.Token, body)
		if err != nil {
			return err
		}
	}

	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("non-OK response from Central: %s - %s", result.Status, string(result.Body))
	}

	if out != nil && len(result.Body) > 0 {
		if err := json.Unmarshal(result.Body, out); err != nil {
			return fmt.Errorf("failed to decode %s response body: %w", method, err)
		}
	}

	return nil
}

func (c *CentralClient) doJSON(method string, requestURL string, token string, body interface{}) (*centralHTTPResponse, error) {
	var payloadBytes []byte
	var err error

	if body != nil {
		payloadBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JSON request body: %w", err)
		}
	}

	var bodyReader io.Reader
	if payloadBytes != nil {
		bodyReader = bytes.NewReader(payloadBytes)
	}

	req, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s request: %w", method, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute %s request: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s response body: %w", method, err)
	}

	return &centralHTTPResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       respBody,
	}, nil
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
