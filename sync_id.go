package main

import (
	"fmt"

	"github.com/google/uuid"
)

func newSyncID() (uuid.UUID, error) {
	syncID, err := uuid.NewRandom()
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to generate sync ID: %w", err)
	}

	return syncID, nil
}
