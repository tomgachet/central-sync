package main

import (
	"strings"
	"testing"
)

func testSubmissionFormTables() []FormTable {
	return []FormTable{
		{ODataName: "Submissions", SQLName: "submissions", IsRoot: true},
		{ODataName: "Submissions.children", SQLName: "submissions__children", IsRoot: false},
		{ODataName: "Submissions.children.details", SQLName: "submissions__children__details", IsRoot: false},
	}
}

func testParsedFormMetadata(formTables []FormTable) ParsedFormMetadata {
	metadata := ParsedFormMetadata{Tables: make(map[string]FormTableSchema)}
	for _, formTable := range formTables {
		metadata.Tables[formTable.ODataName] = FormTableSchema{ODataName: formTable.ODataName}
	}
	return metadata
}

func TestBuildSubmissionBatchesGroupsRowsByRootSubmission(t *testing.T) {
	formTables := testSubmissionFormTables()
	metadata := testParsedFormMetadata(formTables)
	rowsByTable := map[string][]map[string]interface{}{
		"Submissions": {
			{"__id": "uuid:root-b", "name": "second"},
			{"__id": "uuid:root-a", "name": "first"},
		},
		"Submissions.children": {
			{"__id": "uuid:child-b", "__Submissions-id": "uuid:root-b", "value": "child second"},
			{"__id": "uuid:child-a", "__Submissions-id": "uuid:root-a", "value": "child first"},
		},
		"Submissions.children.details": {
			{"__id": "uuid:detail-a", "__children-id": "uuid:child-a", "value": "detail first"},
		},
	}

	batches, err := buildSubmissionBatches(formTables, metadata, rowsByTable)
	if err != nil {
		t.Fatalf("buildSubmissionBatches returned error: %v", err)
	}

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}

	if batches[0].RootSubmissionUUID != "root-a" || batches[1].RootSubmissionUUID != "root-b" {
		t.Fatalf("expected batches sorted by root uuid, got %q then %q", batches[0].RootSubmissionUUID, batches[1].RootSubmissionUUID)
	}

	first := batches[0]
	if first.RootRow == nil || first.RootRow.Shape.RowUUID != "root-a" {
		t.Fatalf("expected root-a root row, got %#v", first.RootRow)
	}
	if len(first.Rows) != 3 {
		t.Fatalf("expected 3 rows in first batch, got %d", len(first.Rows))
	}
	if first.Rows[0].Shape.RowUUID != "root-a" {
		t.Fatalf("expected root row first, got %q", first.Rows[0].Shape.RowUUID)
	}
	for _, row := range first.Rows {
		if row.Shape.RootSubmissionUUID != "root-a" {
			t.Fatalf("expected row %s to resolve to root-a, got %q", row.Shape.RowUUID, row.Shape.RootSubmissionUUID)
		}
	}

	second := batches[1]
	if second.RootRow == nil || second.RootRow.Shape.RowUUID != "root-b" {
		t.Fatalf("expected root-b root row, got %#v", second.RootRow)
	}
	if len(second.Rows) != 2 {
		t.Fatalf("expected 2 rows in second batch, got %d", len(second.Rows))
	}
	if second.Rows[0].Shape.RowUUID != "root-b" {
		t.Fatalf("expected root row first, got %q", second.Rows[0].Shape.RowUUID)
	}
}

func TestBuildSubmissionBatchesReturnsErrorWhenMetadataIsMissing(t *testing.T) {
	formTables := testSubmissionFormTables()
	metadata := ParsedFormMetadata{Tables: map[string]FormTableSchema{
		"Submissions": {ODataName: "Submissions"},
	}}

	_, err := buildSubmissionBatches(formTables, metadata, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "missing parsed schema for OData table Submissions.children") {
		t.Fatalf("expected missing metadata error, got %v", err)
	}
}

func TestBuildSubmissionBatchesReturnsErrorWhenParentIsMissing(t *testing.T) {
	formTables := testSubmissionFormTables()
	metadata := testParsedFormMetadata(formTables)
	rowsByTable := map[string][]map[string]interface{}{
		"Submissions": {
			{"__id": "uuid:root-a"},
		},
		"Submissions.children": {
			{"__id": "uuid:child-a", "__Submissions-id": "uuid:missing-root"},
		},
	}

	_, err := buildSubmissionBatches(formTables, metadata, rowsByTable)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "parent row missing-root not found for row child-a") {
		t.Fatalf("expected missing parent error, got %v", err)
	}
}

func TestBuildSubmissionBatchesReturnsErrorWhenRootRowIsMissing(t *testing.T) {
	formTables := testSubmissionFormTables()
	metadata := testParsedFormMetadata(formTables)
	rowsByTable := map[string][]map[string]interface{}{
		"Submissions.children": {
			{"__id": "uuid:orphan-child"},
		},
	}

	_, err := buildSubmissionBatches(formTables, metadata, rowsByTable)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "missing root row for submission orphan-child") {
		t.Fatalf("expected missing root row error, got %v", err)
	}
}
