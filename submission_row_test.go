package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeSubmissionRowForRoot(t *testing.T) {
	geometry := map[string]interface{}{
		"type":        "Point",
		"coordinates": []interface{}{1.2, 3.4},
	}
	row := map[string]interface{}{
		"__id":                          "uuid:root-a",
		"__system":                      map[string]interface{}{"reviewState": "approved"},
		"meta":                          map[string]interface{}{"instanceID": "uuid:root-a"},
		"children@odata.navigationLink": "ignored",
		"name":                          "Alice",
		"age":                           float64(42),
		"active":                        true,
		"empty_group":                   map[string]interface{}{},
		"location":                      geometry,
		"group": map[string]interface{}{
			"nested": map[string]interface{}{
				"value": "flat",
			},
		},
		"choices": []interface{}{"a", "b"},
	}

	shape, err := analyzeSubmissionRow(FormTable{ODataName: "Submissions", IsRoot: true}, row)
	if err != nil {
		t.Fatalf("analyzeSubmissionRow returned error: %v", err)
	}

	if shape.Kind != SubmissionTableRoot {
		t.Fatalf("expected root kind, got %q", shape.Kind)
	}
	if shape.RowUUID != "root-a" {
		t.Fatalf("expected trimmed uuid root-a, got %q", shape.RowUUID)
	}
	if shape.ParentRowUUID != nil {
		t.Fatalf("expected no parent for root row, got %q", *shape.ParentRowUUID)
	}

	expected := map[string]interface{}{
		"name":                 "Alice",
		"age":                  float64(42),
		"active":               true,
		"empty_group":          nil,
		"location":             geometry,
		"group__nested__value": "flat",
		"choices":              `["a","b"]`,
	}
	if !reflect.DeepEqual(shape.FlatProperties, expected) {
		t.Fatalf("unexpected flat properties:\nexpected: %#v\ngot:      %#v", expected, shape.FlatProperties)
	}
}

func TestAnalyzeSubmissionRowForRepeatUsesDirectParentLink(t *testing.T) {
	row := map[string]interface{}{
		"__id":             "uuid:child-a",
		"__Submissions-id": "uuid:root-a",
		"value":            "child",
	}

	shape, err := analyzeSubmissionRow(FormTable{ODataName: "Submissions.children", IsRoot: false}, row)
	if err != nil {
		t.Fatalf("analyzeSubmissionRow returned error: %v", err)
	}

	if shape.Kind != SubmissionTableRepeat {
		t.Fatalf("expected repeat kind, got %q", shape.Kind)
	}
	if shape.ParentRowUUID == nil || *shape.ParentRowUUID != "root-a" {
		t.Fatalf("expected parent root-a, got %#v", shape.ParentRowUUID)
	}
	if _, exists := shape.FlatProperties["__Submissions-id"]; exists {
		t.Fatalf("expected technical parent link to be excluded from flat properties")
	}
}

func TestAnalyzeSubmissionRowForRepeatUsesNestedParentLink(t *testing.T) {
	row := map[string]interface{}{
		"__id":          "uuid:detail-a",
		"__children-id": "uuid:child-a",
		"value":         "detail",
	}

	shape, err := analyzeSubmissionRow(FormTable{ODataName: "Submissions.children.details", IsRoot: false}, row)
	if err != nil {
		t.Fatalf("analyzeSubmissionRow returned error: %v", err)
	}

	if shape.ParentRowUUID == nil || *shape.ParentRowUUID != "child-a" {
		t.Fatalf("expected parent child-a, got %#v", shape.ParentRowUUID)
	}
}

func TestExtractSubmissionRowUUIDRejectsInvalidRows(t *testing.T) {
	tests := []struct {
		name        string
		row         map[string]interface{}
		expectedErr string
	}{
		{
			name:        "missing id",
			row:         map[string]interface{}{},
			expectedErr: "submission row is missing __id",
		},
		{
			name:        "empty id",
			row:         map[string]interface{}{"__id": ""},
			expectedErr: "submission row __id is invalid",
		},
		{
			name:        "non string id",
			row:         map[string]interface{}{"__id": 123},
			expectedErr: "submission row __id is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractSubmissionRowUUID(tt.row)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("expected error containing %q, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestGetSubmissionPropertyColumnsFromRowsReturnsSortedUniqueColumns(t *testing.T) {
	rows := []map[string]interface{}{
		{
			"__id": "uuid:row-1",
			"b":    "value",
			"group": map[string]interface{}{
				"z": "nested",
			},
		},
		{
			"__id": "uuid:row-2",
			"a":    "value",
			"b":    "other",
		},
	}

	columns, err := getSubmissionPropertyColumnsFromRows(FormTable{ODataName: "Submissions", IsRoot: true}, rows)
	if err != nil {
		t.Fatalf("getSubmissionPropertyColumnsFromRows returned error: %v", err)
	}

	expected := []string{"a", "b", "group__z"}
	if !reflect.DeepEqual(columns, expected) {
		t.Fatalf("expected columns %#v, got %#v", expected, columns)
	}
}

func TestHasParentLinkColumn(t *testing.T) {
	if hasParentLinkColumn(FormTableSchema{Columns: []FormColumnSchema{{Name: "name"}}}) {
		t.Fatalf("expected no parent link column")
	}

	if !hasParentLinkColumn(FormTableSchema{Columns: []FormColumnSchema{{Name: "__Submissions-id"}}}) {
		t.Fatalf("expected parent link column")
	}
}
