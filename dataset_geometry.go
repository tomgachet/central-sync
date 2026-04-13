package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

type GeoJSONFeature struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Geometry   map[string]interface{} `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

func getDatasetEntitiesGeoJSON(centralURL, token string, projectID int, datasetName string) (*GeoJSONFeatureCollection, error) {
	url := fmt.Sprintf("%s/v1/projects/%d/datasets/%s/entities.geojson", centralURL, projectID, datasetName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset GeoJSON request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call dataset GeoJSON endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset GeoJSON response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from dataset GeoJSON endpoint: %s - %s", resp.Status, string(body))
	}

	var result GeoJSONFeatureCollection
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode dataset GeoJSON response: %w", err)
	}

	return &result, nil
}

func buildGeometryGeoJSONMap(collection *GeoJSONFeatureCollection) map[string]interface{} {
	result := make(map[string]interface{})

	if collection == nil {
		return result
	}

	for _, feature := range collection.Features {
		if feature.ID == "" || feature.Geometry == nil {
			continue
		}
		result[feature.ID] = feature.Geometry
	}

	return result
}