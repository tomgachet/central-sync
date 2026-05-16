package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type CentralForm struct {
	XMLFormID string  `json:"xmlFormId"`
	Name      string  `json:"name"`
	Version   *string `json:"version"`
	State     string  `json:"state"`
	CreatedAt string  `json:"createdAt"`
}

type ODataServiceDocument struct {
	Context string             `json:"@odata.context"`
	Value   []ODataServiceItem `json:"value"`
}

type ODataServiceItem struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type FormTable struct {
	ODataName string
	ODataURL  string
	SQLName   string
	IsRoot    bool
}

func listProjectForms(client *CentralClient, projectID int) ([]CentralForm, error) {
	url := fmt.Sprintf("%s/v1/projects/%d/forms", client.BaseURL, projectID)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call forms endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read forms response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from forms endpoint: %s - %s", resp.Status, string(body))
	}

	var forms []CentralForm
	if err := json.Unmarshal(body, &forms); err != nil {
		return nil, fmt.Errorf("failed to decode forms response: %w", err)
	}

	return forms, nil
}

func formExists(client *CentralClient, projectID int, xmlFormID string) (bool, error) {
	forms, err := listProjectForms(client, projectID)
	if err != nil {
		return false, err
	}

	for _, form := range forms {
		if form.XMLFormID == xmlFormID {
			return true, nil
		}
	}

	return false, nil
}

func getFormODataServiceDocument(client *CentralClient, projectID int, xmlFormID string) (*ODataServiceDocument, error) {
	url := fmt.Sprintf("%s/v1/projects/%d/forms/%s.svc", client.BaseURL, projectID, xmlFormID)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call form OData service document: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read form OData service document: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from form OData service document: %s - %s", resp.Status, string(body))
	}

	var doc ODataServiceDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("failed to decode form OData service document: %w", err)
	}

	return &doc, nil
}

func getFormTables(client *CentralClient, projectID int, xmlFormID string, rootTableName string) ([]FormTable, error) {
	doc, err := getFormODataServiceDocument(client, projectID, xmlFormID)
	if err != nil {
		return nil, err
	}

	var tables []FormTable

	for _, item := range doc.Value {
		if item.Kind != "EntitySet" {
			continue
		}

		sqlName, isRoot, err := buildFormTableSQLName(rootTableName, item.Name)
		if err != nil {
			return nil, err
		}

		tables = append(tables, FormTable{
			ODataName: item.Name,
			ODataURL:  item.URL,
			SQLName:   sqlName,
			IsRoot:    isRoot,
		})
	}

	return tables, nil
}

func buildFormTableSQLName(rootTableName string, odataName string) (string, bool, error) {
	if odataName == "Submissions" {
		return rootTableName, true, nil
	}

	prefix := "Submissions."
	if !strings.HasPrefix(odataName, prefix) {
		return "", false, fmt.Errorf("unsupported OData table name: %s", odataName)
	}

	suffix := strings.TrimPrefix(odataName, prefix)
	suffix = strings.ReplaceAll(suffix, ".", "__")

	return rootTableName + "__" + suffix, false, nil
}