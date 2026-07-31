package main

import "fmt"

type SubmissionAttachmentSource struct {
	ODataTableName string
	SQLTableName   string
	SourceRowUUID  string
	FieldName      string
}

func findSubmissionAttachmentSource(batch *SubmissionBatch, filename string) (*SubmissionAttachmentSource, error) {
	if batch == nil || filename == "" {
		return nil, nil
	}

	var source *SubmissionAttachmentSource
	for _, row := range batch.Rows {
		if row == nil || row.Shape == nil {
			continue
		}
		for fieldName, rawValue := range row.Shape.FlatProperties {
			fieldValue, ok := rawValue.(string)
			if !ok || fieldValue != filename {
				continue
			}
			if source != nil {
				return nil, fmt.Errorf("attachment %q is referenced by more than one submission field", filename)
			}
			source = &SubmissionAttachmentSource{
				ODataTableName: row.FormTable.ODataName,
				SQLTableName:   row.FormTable.SQLName,
				SourceRowUUID:  row.Shape.RowUUID,
				FieldName:      fieldName,
			}
		}
	}

	return source, nil
}
