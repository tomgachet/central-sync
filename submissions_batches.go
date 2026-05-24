package main

import "fmt"

type SubmissionRowRef struct {
	FormTable   FormTable
	TableSchema FormTableSchema
	Row         map[string]interface{}
	Shape       *SubmissionRowShape
}

type SubmissionBatch struct {
	RootSubmissionUUID string
	RootRow            *SubmissionRowRef
	Rows               []*SubmissionRowRef
}

func buildSubmissionBatches(
	formTables []FormTable,
	parsedMetadata ParsedFormMetadata,
	rowsByTable map[string][]map[string]interface{},
) ([]*SubmissionBatch, error) {
	var allRows []*SubmissionRowRef
	rowByUUID := make(map[string]*SubmissionRowRef)

	for _, formTable := range formTables {
		tableSchema, ok := parsedMetadata.Tables[formTable.ODataName]
		if !ok {
			return nil, fmt.Errorf("missing parsed schema for OData table %s", formTable.ODataName)
		}

		rows := rowsByTable[formTable.ODataName]

		for _, row := range rows {
			shape, err := analyzeSubmissionRow(formTable, row)
			if err != nil {
				return nil, fmt.Errorf("failed to analyze row for table %s: %w", formTable.ODataName, err)
			}

			rowRef := &SubmissionRowRef{
				FormTable:   formTable,
				TableSchema: tableSchema,
				Row:         row,
				Shape:       shape,
			}

			allRows = append(allRows, rowRef)
			rowByUUID[shape.RowUUID] = rowRef
		}
	}

	for _, rowRef := range allRows {
		rootUUID, err := resolveRootSubmissionUUID(rowRef, rowByUUID)
		if err != nil {
			return nil, err
		}
		rowRef.Shape.RootSubmissionUUID = rootUUID
	}

	batchByRoot := make(map[string]*SubmissionBatch)

	for _, rowRef := range allRows {
		rootUUID := rowRef.Shape.RootSubmissionUUID
		if rootUUID == "" {
			return nil, fmt.Errorf("empty root submission uuid for row %s", rowRef.Shape.RowUUID)
		}

		batch, ok := batchByRoot[rootUUID]
		if !ok {
			batch = &SubmissionBatch{
				RootSubmissionUUID: rootUUID,
			}
			batchByRoot[rootUUID] = batch
		}

		batch.Rows = append(batch.Rows, rowRef)

		if rowRef.FormTable.IsRoot {
			batch.RootRow = rowRef
		}
	}

	var batches []*SubmissionBatch
	for _, batch := range batchByRoot {
		if batch.RootRow == nil {
			return nil, fmt.Errorf("missing root row for submission %s", batch.RootSubmissionUUID)
		}

		batch.Rows = sortSubmissionBatchRows(batch.Rows)
		batches = append(batches, batch)
	}

	batches = sortSubmissionBatches(batches)
	return batches, nil
}

func resolveRootSubmissionUUID(
	rowRef *SubmissionRowRef,
	rowByUUID map[string]*SubmissionRowRef,
) (string, error) {
	if rowRef == nil || rowRef.Shape == nil {
		return "", fmt.Errorf("invalid submission row reference")
	}

	current := rowRef
	visited := make(map[string]bool)

	for {
		if current.Shape == nil {
			return "", fmt.Errorf("row %s has no shape", rowRef.Shape.RowUUID)
		}

		if visited[current.Shape.RowUUID] {
			return "", fmt.Errorf("cyclic parent chain detected from row %s", rowRef.Shape.RowUUID)
		}
		visited[current.Shape.RowUUID] = true

		if current.Shape.ParentRowUUID == nil || *current.Shape.ParentRowUUID == "" {
			return current.Shape.RowUUID, nil
		}

		parent, ok := rowByUUID[*current.Shape.ParentRowUUID]
		if !ok {
			return "", fmt.Errorf(
				"parent row %s not found for row %s",
				*current.Shape.ParentRowUUID,
				current.Shape.RowUUID,
			)
		}

		current = parent
	}
}

func sortSubmissionBatchRows(rows []*SubmissionRowRef) []*SubmissionRowRef {
	priority := func(row *SubmissionRowRef) int {
		if row.FormTable.IsRoot {
			return 0
		}
		if row.Shape.ParentRowUUID != nil {
			return 1
		}
		return 2
	}

	sorted := make([]*SubmissionRowRef, len(rows))
	copy(sorted, rows)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if priority(sorted[j]) < priority(sorted[i]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

func sortSubmissionBatches(batches []*SubmissionBatch) []*SubmissionBatch {
	sorted := make([]*SubmissionBatch, len(batches))
	copy(sorted, batches)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].RootSubmissionUUID < sorted[i].RootSubmissionUUID {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}