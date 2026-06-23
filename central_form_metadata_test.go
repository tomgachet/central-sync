package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestMapODataTypeToSQLType(t *testing.T) {
	tests := []struct {
		odataType string
		expected  string
	}{
		{odataType: "Edm.String", expected: "TEXT"},
		{odataType: "Edm.Boolean", expected: "BOOLEAN"},
		{odataType: "Edm.Int64", expected: "BIGINT"},
		{odataType: "Edm.Int32", expected: "INT"},
		{odataType: "Edm.Decimal", expected: "DOUBLE PRECISION"},
		{odataType: "Edm.Double", expected: "DOUBLE PRECISION"},
		{odataType: "Edm.DateTimeOffset", expected: "TIMESTAMPTZ"},
		{odataType: "Edm.Date", expected: "DATE"},
		{odataType: "Edm.GeographyPoint", expected: "JSONB"},
		{odataType: "Edm.GeographyLineString", expected: "JSONB"},
		{odataType: "Edm.GeographyPolygon", expected: "JSONB"},
		{odataType: "Collection(Edm.String)", expected: "JSONB"},
		{odataType: "Unknown.Type", expected: "TEXT"},
	}

	for _, tt := range tests {
		t.Run(tt.odataType, func(t *testing.T) {
			if got := mapODataTypeToSQLType(tt.odataType); got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestParseFormMetadataFlattensEntityTypes(t *testing.T) {
	doc := &EDMXDocument{
		DataServices: EDMXDataServices{
			Schema: EDMXSchema{
				Namespace: "org.opendatakit.user.demo",
				EntityContainer: EDMXEntityContainer{EntitySets: []EDMXEntitySetEntry{
					{Name: "Submissions", EntityType: "org.opendatakit.user.demo.Submissions"},
					{Name: "Submissions.children", EntityType: "org.opendatakit.user.demo.SubmissionsChildren"},
				}},
				EntityTypes: []EDMXEntityType{
					{
						Name: "Submissions",
						Properties: []EDMXProperty{
							{Name: "__id", Type: "Edm.String"},
							{Name: "__system", Type: "org.opendatakit.user.demo.System"},
							{Name: "meta", Type: "org.opendatakit.user.demo.Meta"},
							{Name: "name", Type: "Edm.String"},
							{Name: "age", Type: "Edm.Int32"},
							{Name: "location", Type: "Edm.GeographyPoint"},
							{Name: "group", Type: "org.opendatakit.user.demo.Group"},
						},
					},
					{
						Name: "SubmissionsChildren",
						Properties: []EDMXProperty{
							{Name: "__id", Type: "Edm.String"},
							{Name: "__Submissions-id", Type: "Edm.String"},
							{Name: "child_value", Type: "Edm.Boolean"},
						},
					},
				},
				ComplexTypes: []EDMXComplexType{
					{
						Name: "Group",
						Properties: []EDMXProperty{
							{Name: "nested", Type: "org.opendatakit.user.demo.Nested"},
							{Name: "score", Type: "Edm.Decimal"},
						},
					},
					{
						Name: "Nested",
						Properties: []EDMXProperty{
							{Name: "value", Type: "Edm.String"},
						},
					},
				},
			},
		},
	}

	metadata, err := parseFormMetadata(doc, []FormTable{
		{ODataName: "Submissions", IsRoot: true},
		{ODataName: "Submissions.children"},
	})
	if err != nil {
		t.Fatalf("parseFormMetadata returned error: %v", err)
	}

	rootColumns := metadata.Tables["Submissions"].Columns
	expectedRootColumns := []FormColumnSchema{
		{Name: "name", ODataType: "Edm.String", SQLType: "TEXT"},
		{Name: "age", ODataType: "Edm.Int32", SQLType: "INT"},
		{Name: "location", ODataType: "Edm.GeographyPoint", SQLType: "JSONB"},
		{Name: "group__nested__value", ODataType: "Edm.String", SQLType: "TEXT"},
		{Name: "group__score", ODataType: "Edm.Decimal", SQLType: "DOUBLE PRECISION"},
	}
	if !reflect.DeepEqual(rootColumns, expectedRootColumns) {
		t.Fatalf("unexpected root columns:\nexpected: %#v\ngot:      %#v", expectedRootColumns, rootColumns)
	}

	childColumns := metadata.Tables["Submissions.children"].Columns
	expectedChildColumns := []FormColumnSchema{
		{Name: "__Submissions-id", ODataType: "Edm.String", SQLType: "TEXT"},
		{Name: "child_value", ODataType: "Edm.Boolean", SQLType: "BOOLEAN"},
	}
	if !reflect.DeepEqual(childColumns, expectedChildColumns) {
		t.Fatalf("unexpected child columns:\nexpected: %#v\ngot:      %#v", expectedChildColumns, childColumns)
	}
}

func TestParseFormMetadataReturnsErrorsForInvalidMetadata(t *testing.T) {
	tests := []struct {
		name        string
		doc         *EDMXDocument
		formTables  []FormTable
		expectedErr string
	}{
		{
			name:        "nil document",
			doc:         nil,
			formTables:  []FormTable{{ODataName: "Submissions"}},
			expectedErr: "metadata document is nil",
		},
		{
			name: "missing entity set",
			doc: &EDMXDocument{DataServices: EDMXDataServices{Schema: EDMXSchema{
				Namespace: "demo",
			}}},
			formTables:  []FormTable{{ODataName: "Submissions"}},
			expectedErr: "entity set Submissions not found in metadata",
		},
		{
			name: "missing entity type",
			doc: &EDMXDocument{DataServices: EDMXDataServices{Schema: EDMXSchema{
				Namespace: "demo",
				EntityContainer: EDMXEntityContainer{EntitySets: []EDMXEntitySetEntry{
					{Name: "Submissions", EntityType: "demo.MissingType"},
				}},
			}}},
			formTables:  []FormTable{{ODataName: "Submissions"}},
			expectedErr: "entity type MissingType not found for OData table Submissions",
		},
		{
			name: "unsupported complex type",
			doc: &EDMXDocument{DataServices: EDMXDataServices{Schema: EDMXSchema{
				Namespace: "demo",
				EntityContainer: EDMXEntityContainer{EntitySets: []EDMXEntitySetEntry{
					{Name: "Submissions", EntityType: "demo.Submissions"},
				}},
				EntityTypes: []EDMXEntityType{
					{Name: "Submissions", Properties: []EDMXProperty{{Name: "group", Type: "demo.UnknownComplex"}}},
				},
			}}},
			formTables:  []FormTable{{ODataName: "Submissions"}},
			expectedErr: "unsupported non-primitive property type demo.UnknownComplex for column group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFormMetadata(tt.doc, tt.formTables)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("expected error containing %q, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestTrimNamespace(t *testing.T) {
	if got := trimNamespace("demo.Submissions", "demo"); got != "Submissions" {
		t.Fatalf("expected trimmed namespace, got %q", got)
	}
	if got := trimNamespace("other.Submissions", "demo"); got != "other.Submissions" {
		t.Fatalf("expected unmatched namespace to stay unchanged, got %q", got)
	}
}
