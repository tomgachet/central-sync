package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type SubmissionTableKind string

const (
	SubmissionTableRoot   SubmissionTableKind = "root"
	SubmissionTableRepeat SubmissionTableKind = "repeat"
)

type SubmissionRowShape struct {
	Kind               SubmissionTableKind
	RowUUID            string
	ParentRowUUID      *string
	RootSubmissionUUID string
	FlatProperties     map[string]interface{}
}

func detectSubmissionTableKind(formTable FormTable) SubmissionTableKind {
	if formTable.IsRoot {
		return SubmissionTableRoot
	}
	return SubmissionTableRepeat
}

func analyzeSubmissionRow(formTable FormTable, row map[string]interface{}) (*SubmissionRowShape, error) {
	rowUUID, err := extractSubmissionRowUUID(row)
	if err != nil {
		return nil, err
	}

	kind := detectSubmissionTableKind(formTable)

	var parentRowUUID *string
	rootSubmissionUUID := rowUUID

	if kind == SubmissionTableRepeat {
		parentRowUUID = extractSubmissionParentRowUUID(row)
		if parentRowUUID != nil && *parentRowUUID != "" {
			rootSubmissionUUID = *parentRowUUID
		}
	}

	flatProperties := flattenSubmissionProperties(row)

	return &SubmissionRowShape{
		Kind:               kind,
		RowUUID:            rowUUID,
		ParentRowUUID:      parentRowUUID,
		RootSubmissionUUID: rootSubmissionUUID,
		FlatProperties:     flatProperties,
	}, nil
}

func extractSubmissionRowUUID(row map[string]interface{}) (string, error) {
	raw, ok := row["__id"]
	if !ok || raw == nil {
		return "", fmt.Errorf("submission row is missing __id")
	}

	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("submission row __id is invalid")
	}

	return trimUUIDPrefix(value), nil
}

func extractSubmissionParentRowUUID(row map[string]interface{}) *string {
	var candidates []string

	for key, raw := range row {
		if key == "__id" {
			continue
		}

		if !strings.HasPrefix(key, "__") || !strings.HasSuffix(key, "-id") {
			continue
		}

		value, ok := raw.(string)
		if !ok || value == "" {
			continue
		}

		candidates = append(candidates, trimUUIDPrefix(value))
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Strings(candidates)
	parent := candidates[0]
	return &parent
}

func flattenSubmissionProperties(row map[string]interface{}) map[string]interface{} {
	flat := make(map[string]interface{})

	for key, value := range row {
		if isSubmissionTechnicalSourceKey(key) {
			continue
		}

		flattenSubmissionValue(flat, key, value)
	}

	return flat
}

func flattenSubmissionValue(flat map[string]interface{}, prefix string, value interface{}) {
	if value == nil {
		flat[prefix] = nil
		return
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		if len(typed) == 0 {
			flat[prefix] = nil
			return
		}

		if isGeoJSONLikeObject(typed) {
			flat[prefix] = typed
			return
		}

		for childKey, childValue := range typed {
			nextKey := prefix + "__" + childKey
			flattenSubmissionValue(flat, nextKey, childValue)
		}

	case []interface{}:
		bytes, err := json.Marshal(typed)
		if err != nil {
			flat[prefix] = fmt.Sprintf("%v", typed)
			return
		}
		flat[prefix] = string(bytes)

	case string:
		flat[prefix] = typed

	case bool:
		flat[prefix] = typed

	case float64:
		flat[prefix] = typed

	default:
		bytes, err := json.Marshal(typed)
		if err != nil {
			flat[prefix] = fmt.Sprintf("%v", typed)
			return
		}
		flat[prefix] = string(bytes)
	}
}

func isGeoJSONLikeObject(value map[string]interface{}) bool {
	_, hasType := value["type"]
	_, hasCoordinates := value["coordinates"]

	return hasType && hasCoordinates
}

func isSubmissionTechnicalSourceKey(key string) bool {
	if key == "__id" || key == "__system" || key == "meta" {
		return true
	}

	if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "-id") {
		return true
	}

	if strings.HasSuffix(key, "@odata.navigationLink") {
		return true
	}

	return false
}

func getSubmissionPropertyColumnsFromRows(formTable FormTable, rows []map[string]interface{}) ([]string, error) {
	seen := make(map[string]bool)
	var columns []string

	for _, row := range rows {
		shape, err := analyzeSubmissionRow(formTable, row)
		if err != nil {
			return nil, err
		}

		for column := range shape.FlatProperties {
			if column == "" || seen[column] {
				continue
			}

			seen[column] = true
			columns = append(columns, column)
		}
	}

	sort.Strings(columns)
	return columns, nil
}

func hasParentLinkColumn(tableSchema FormTableSchema) bool {
	for _, column := range tableSchema.Columns {
		if isSubmissionLinkColumn(column.Name) {
			return true
		}
	}
	return false
}