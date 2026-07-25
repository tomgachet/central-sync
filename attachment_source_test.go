package main

import (
	"strings"
	"testing"
)

func TestFindSubmissionAttachmentSourceFindsRepeatField(t *testing.T) {
	batch := &SubmissionBatch{Rows: []*SubmissionRowRef{
		{FormTable: FormTable{ODataName: "Submissions", SQLName: "site_visit", IsRoot: true}, Shape: &SubmissionRowShape{RowUUID: "root-row", FlatProperties: map[string]interface{}{"name": "Alice"}}},
		{FormTable: FormTable{ODataName: "Submissions.observations", SQLName: "site_visit__observations"}, Shape: &SubmissionRowShape{RowUUID: "repeat-row", FlatProperties: map[string]interface{}{"repeat_photo": "repeat.jpg"}}},
	}}

	source, err := findSubmissionAttachmentSource(batch, "repeat.jpg")
	if err != nil {
		t.Fatalf("findSubmissionAttachmentSource returned error: %v", err)
	}
	if source == nil || source.ODataTableName != "Submissions.observations" || source.SourceRowUUID != "repeat-row" || source.FieldName != "repeat_photo" {
		t.Fatalf("unexpected attachment source: %#v", source)
	}
}

func TestFindSubmissionAttachmentSourceReturnsNilWhenFieldIsUnknown(t *testing.T) {
	source, err := findSubmissionAttachmentSource(&SubmissionBatch{}, "audit.csv")
	if err != nil || source != nil {
		t.Fatalf("expected no source, got source=%#v error=%v", source, err)
	}
}

func TestFindSubmissionAttachmentSourceRejectsAmbiguousFilename(t *testing.T) {
	batch := &SubmissionBatch{Rows: []*SubmissionRowRef{
		{FormTable: FormTable{ODataName: "Submissions", SQLName: "form"}, Shape: &SubmissionRowShape{RowUUID: "root", FlatProperties: map[string]interface{}{"photo": "same.jpg"}}},
		{FormTable: FormTable{ODataName: "Submissions.repeat", SQLName: "form__repeat"}, Shape: &SubmissionRowShape{RowUUID: "repeat", FlatProperties: map[string]interface{}{"photo": "same.jpg"}}},
	}}

	_, err := findSubmissionAttachmentSource(batch, "same.jpg")
	if err == nil || !strings.Contains(err.Error(), "more than one submission field") {
		t.Fatalf("expected ambiguous attachment error, got %v", err)
	}
}
