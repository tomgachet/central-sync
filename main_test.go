package main

import (
	"os"
	"testing"
)

func TestRunExitStatus(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if got := run([]string{"--version"}); got != 0 {
			t.Fatalf("run(--version) = %d, want 0", got)
		}
	})

	t.Run("missing environment file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if got := run(nil); got != 2 {
			t.Fatalf("run with missing .env = %d, want 2", got)
		}
	})

	t.Run("invalid project configuration", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := os.WriteFile(".env", nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("central_config.yaml", []byte("projects: ["), 0600); err != nil {
			t.Fatal(err)
		}
		if got := run(nil); got != 2 {
			t.Fatalf("run with invalid configuration = %d, want 2", got)
		}
	})
}
