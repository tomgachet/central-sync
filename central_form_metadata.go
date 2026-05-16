package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type EDMXDocument struct {
	DataServices EDMXDataServices `xml:"DataServices"`
}

type EDMXDataServices struct {
	Schema EDMXSchema `xml:"Schema"`
}

type EDMXSchema struct {
	Namespace       string              `xml:"Namespace,attr"`
	EntityTypes     []EDMXEntityType    `xml:"EntityType"`
	ComplexTypes    []EDMXComplexType   `xml:"ComplexType"`
	EntityContainer EDMXEntityContainer `xml:"EntityContainer"`
}

type EDMXEntityContainer struct {
	Name       string               `xml:"Name,attr"`
	EntitySets []EDMXEntitySetEntry `xml:"EntitySet"`
}

type EDMXEntitySetEntry struct {
	Name       string `xml:"Name,attr"`
	EntityType string `xml:"EntityType,attr"`
}

type EDMXEntityType struct {
	Name       string            `xml:"Name,attr"`
	Properties []EDMXProperty    `xml:"Property"`
	Navigation []EDMXNavProperty `xml:"NavigationProperty"`
}

type EDMXComplexType struct {
	Name       string         `xml:"Name,attr"`
	Properties []EDMXProperty `xml:"Property"`
}

type EDMXProperty struct {
	Name string `xml:"Name,attr"`
	Type string `xml:"Type,attr"`
}

type EDMXNavProperty struct {
	Name string `xml:"Name,attr"`
	Type string `xml:"Type,attr"`
}

type FormColumnSchema struct {
	Name      string
	ODataType string
	SQLType   string
}

type FormTableSchema struct {
	ODataName string
	Columns   []FormColumnSchema
}

type ParsedFormMetadata struct {
	Tables map[string]FormTableSchema
}

func getFormMetadataDocument(client *CentralClient, projectID int, xmlFormID string) (*EDMXDocument, error) {
	url := fmt.Sprintf("%s/v1/projects/%d/forms/%s.svc/$metadata", client.BaseURL, projectID, xmlFormID)

	resp, err := client.GetWithAccept(url, "application/xml")
	if err != nil {
		return nil, fmt.Errorf("failed to call form metadata endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read form metadata response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-OK response from form metadata endpoint: %s - %s", resp.Status, string(body))
	}

	var doc EDMXDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("failed to decode form metadata XML: %w", err)
	}

	return &doc, nil
}

func parseFormMetadata(doc *EDMXDocument, formTables []FormTable) (*ParsedFormMetadata, error) {
	if doc == nil {
		return nil, fmt.Errorf("metadata document is nil")
	}

	schema := doc.DataServices.Schema

	entityTypes := make(map[string]EDMXEntityType)
	for _, entityType := range schema.EntityTypes {
		entityTypes[entityType.Name] = entityType
	}

	complexTypes := make(map[string]EDMXComplexType)
	for _, complexType := range schema.ComplexTypes {
		complexTypes[complexType.Name] = complexType
	}

	entitySetTypes := make(map[string]string)
	for _, entitySet := range schema.EntityContainer.EntitySets {
		entitySetTypes[entitySet.Name] = trimNamespace(entitySet.EntityType, schema.Namespace)
	}

	result := &ParsedFormMetadata{
		Tables: make(map[string]FormTableSchema),
	}

	for _, formTable := range formTables {
		entityTypeName, ok := entitySetTypes[formTable.ODataName]
		if !ok {
			return nil, fmt.Errorf("entity set %s not found in metadata", formTable.ODataName)
		}

		entityType, ok := entityTypes[entityTypeName]
		if !ok {
			return nil, fmt.Errorf("entity type %s not found for OData table %s", entityTypeName, formTable.ODataName)
		}

		columns, err := flattenEntityTypeColumns(schema.Namespace, entityType, complexTypes, "")
		if err != nil {
			return nil, err
		}

		result.Tables[formTable.ODataName] = FormTableSchema{
			ODataName: formTable.ODataName,
			Columns:   columns,
		}
	}

	return result, nil
}

func flattenEntityTypeColumns(
	namespace string,
	entityType EDMXEntityType,
	complexTypes map[string]EDMXComplexType,
	prefix string,
) ([]FormColumnSchema, error) {
	var columns []FormColumnSchema

	for _, property := range entityType.Properties {
		if prefix == "" && isIgnoredMetadataProperty(property.Name) {
			continue
		}

		flatColumns, err := flattenProperty(namespace, property, complexTypes, prefix)
		if err != nil {
			return nil, err
		}
		columns = append(columns, flatColumns...)
	}

	return columns, nil
}

func flattenComplexTypeColumns(
	namespace string,
	complexType EDMXComplexType,
	complexTypes map[string]EDMXComplexType,
	prefix string,
) ([]FormColumnSchema, error) {
	var columns []FormColumnSchema

	for _, property := range complexType.Properties {
		if prefix == "" && isIgnoredMetadataProperty(property.Name) {
			continue
		}

		flatColumns, err := flattenProperty(namespace, property, complexTypes, prefix)
		if err != nil {
			return nil, err
		}
		columns = append(columns, flatColumns...)
	}

	return columns, nil
}

func flattenProperty(
	namespace string,
	property EDMXProperty,
	complexTypes map[string]EDMXComplexType,
	prefix string,
) ([]FormColumnSchema, error) {
	columnName := property.Name
	if prefix != "" {
		columnName = prefix + "__" + property.Name
	}

	if isEDMPrimitiveType(property.Type) {
		return []FormColumnSchema{
			{
				Name:      columnName,
				ODataType: property.Type,
				SQLType:   mapODataTypeToSQLType(property.Type),
			},
		}, nil
	}

	complexTypeName := trimNamespace(property.Type, namespace)
	complexType, ok := complexTypes[complexTypeName]
	if !ok {
		return nil, fmt.Errorf("unsupported non-primitive property type %s for column %s", property.Type, columnName)
	}

	return flattenComplexTypeColumns(namespace, complexType, complexTypes, columnName)
}

func isIgnoredMetadataProperty(propertyName string) bool {
	switch propertyName {
	case "__id", "__system", "meta":
		return true
	default:
		return false
	}
}

func isEDMPrimitiveType(odataType string) bool {
	return strings.HasPrefix(odataType, "Edm.") || strings.HasPrefix(odataType, "Collection(Edm.")
}

func trimNamespace(typeName string, namespace string) string {
	prefix := namespace + "."
	return strings.TrimPrefix(typeName, prefix)
}

func mapODataTypeToSQLType(odataType string) string {
	switch odataType {
	case "Edm.String":
		return "TEXT"
	case "Edm.Boolean":
		return "BOOLEAN"
	case "Edm.Int64":
		return "BIGINT"
	case "Edm.Int32":
		return "INT"
	case "Edm.Decimal", "Edm.Double":
		return "DOUBLE PRECISION"
	case "Edm.DateTimeOffset":
		return "TIMESTAMPTZ"
	case "Edm.Date":
		return "DATE"
	case "Edm.GeographyPoint", "Edm.GeographyLineString", "Edm.GeographyPolygon":
		return "JSONB"
	default:
		if strings.HasPrefix(odataType, "Collection(") {
			return "JSONB"
		}
		return "TEXT"
	}
}