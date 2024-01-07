package gotemplates

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/kayac/go-katsubushi/v2"
	"github.com/oklog/ulid/v2"
)

// NewUUID len: 36
func NewUUID() string {
	return uuid.New().String()
}

// NewULID len: 26
func NewULID() string {
	return ulid.Make().String()
}

// NewIDGenerator katsubushi
func NewIDGenerator() (katsubushi.Generator, error) {
	workerIDStr := GetEnv("WORKER_ID", "1")
	workerID, err := strconv.ParseUint(workerIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WORKER_ID: %w", err)
	}
	idg, err := katsubushi.NewGenerator(uint(workerID))
	if err != nil {
		return nil, fmt.Errorf("failed to create katsubushi generator: %w", err)
	}
	return idg, nil
}

func GenerateID(idg katsubushi.Generator) (uint64, error) {
	return idg.NextID()
}
