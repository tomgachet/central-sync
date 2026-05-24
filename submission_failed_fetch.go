package main

import "fmt"

func fetchFailedSubmissionRowsByTable(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	formTables []FormTable,
	failedSubmissionUUIDs []string,
) (map[string][]map[string]interface{}, error) {
	rowsByTable := make(map[string][]map[string]interface{})

	if len(failedSubmissionUUIDs) == 0 {
		return rowsByTable, nil
	}

	rootTable := findRootFormTable(formTables)
	if rootTable == nil {
		return nil, fmt.Errorf("root form table not found")
	}

	rootRows, err := getSubmissionRowsByUUIDs(
		client,
		projectID,
		xmlFormID,
		failedSubmissionUUIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch failed root submissions: %w", err)
	}
	rowsByTable[rootTable.ODataName] = rootRows

	parentRowsByTable := map[string][]map[string]interface{}{
		rootTable.ODataName: rootRows,
	}

	for _, formTable := range formTables {
		if formTable.IsRoot {
			continue
		}

		parentTableName := getParentODataName(formTable.ODataName)
		parentRows := parentRowsByTable[parentTableName]
		if len(parentRows) == 0 {
			rowsByTable[formTable.ODataName] = nil
			parentRowsByTable[formTable.ODataName] = nil
			continue
		}

		rows, err := getChildRowsFromParentNavigationLinks(
			client,
			projectID,
			xmlFormID,
			formTable,
			parentRows,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch failed rows for %s: %w", formTable.ODataName, err)
		}

		rowsByTable[formTable.ODataName] = rows
		parentRowsByTable[formTable.ODataName] = rows
	}

	return rowsByTable, nil
}

func findRootFormTable(formTables []FormTable) *FormTable {
	for i := range formTables {
		if formTables[i].IsRoot {
			return &formTables[i]
		}
	}
	return nil
}

func getParentODataName(odataName string) string {
	lastDot := -1
	for i := len(odataName) - 1; i >= 0; i-- {
		if odataName[i] == '.' {
			lastDot = i
			break
		}
	}
	if lastDot == -1 {
		return ""
	}
	return odataName[:lastDot]
}

func getChildSegmentName(odataName string) string {
	lastDot := -1
	for i := len(odataName) - 1; i >= 0; i-- {
		if odataName[i] == '.' {
			lastDot = i
			break
		}
	}
	if lastDot == -1 {
		return odataName
	}
	return odataName[lastDot+1:]
}

func getChildRowsFromParentNavigationLinks(
	client *CentralClient,
	projectID int,
	xmlFormID string,
	formTable FormTable,
	parentRows []map[string]interface{},
) ([]map[string]interface{}, error) {
	childSegment := getChildSegmentName(formTable.ODataName)
	navKey := childSegment + "@odata.navigationLink"

	rowByUUID := make(map[string]map[string]interface{})

	for _, parentRow := range parentRows {
		rawLink, ok := parentRow[navKey]
		if !ok || rawLink == nil {
			continue
		}

		link, ok := rawLink.(string)
		if !ok || link == "" {
			continue
		}

		rows, err := getAllFormTableRows(
			client,
			projectID,
			xmlFormID,
			link,
			"",
		)
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			rowUUID, err := extractSubmissionRowUUID(row)
			if err != nil || rowUUID == "" {
				continue
			}
			rowByUUID[rowUUID] = row
		}
	}

	var result []map[string]interface{}
	for _, row := range rowByUUID {
		result = append(result, row)
	}

	return result, nil
}