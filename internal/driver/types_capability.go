package driver

import "context"

// SequenceManager is the capability extension for sequence CRUD
// (spec §4.3; PostgreSQL in v0.1). Capability endpoints return
// driver_not_supported when the session driver does not implement it.
type SequenceManager interface {
	ListSequences(ctx context.Context, schema string) ([]SequenceInfo, error)
	CreateSequence(ctx context.Context, name string, increment int64) error
	DropSequence(ctx context.Context, name string) error
	AlterSequence(ctx context.Context, name string, restartWith, increment *int64) error
}

// SequenceInfo describes one sequence (the TS mirror lives in
// frontend/src/api/types.ts).
type SequenceInfo struct {
	Name      string `json:"name"`
	LastValue int64  `json:"lastValue"`
	Increment int64  `json:"increment"`
}
