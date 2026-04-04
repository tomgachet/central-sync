package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func testCentralProjects(envPath, token string) error {
	err := loadEnvFile(envPath)
	if err != nil {
		return fmt.Errorf("failed to load .env file: %w", err)
	}

	centralURL := os.Getenv("ODK_CENTRAL_URL")
	if centralURL == "" {
		return fmt.Errorf("missing ODK_CENTRAL_URL")
	}

	return listProjects(centralURL, token)
}

func listProjects(centralURL, token string) error {
	url := centralURL + "/v1/projects"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create projects request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call projects endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read projects response: %w", err)
	}

	fmt.Println("Projects status:", resp.Status)
	fmt.Println("Projects response body:")
	fmt.Println(string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("KO response from /v1/projects: %s", resp.Status)
	}

	return nil
}