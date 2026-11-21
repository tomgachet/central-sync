package main

import (
	"errors"
	"testing"
)

func TestSyncOutcomeExitCode(t *testing.T) {
	syncErr := errors.New("sync failed")
	type result struct {
		completed bool
		err       error
	}
	for _, test := range []struct {
		name    string
		results []result
		want    int
	}{
		{"all succeeded", []result{{true, nil}, {true, nil}}, 0},
		{"success after failure", []result{{false, syncErr}, {true, nil}}, 1},
		{"failure after success", []result{{true, nil}, {false, syncErr}}, 1},
		{"error after commit", []result{{true, syncErr}}, 1},
		{"all failed", []result{{false, syncErr}, {false, syncErr}}, 2},
		{"no enabled work", nil, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var outcome syncOutcome
			for _, result := range test.results {
				outcome.record(result.completed, result.err)
			}
			if got := outcome.exitCode(); got != test.want {
				t.Fatalf("exit status = %d, want %d", got, test.want)
			}
		})
	}
}
