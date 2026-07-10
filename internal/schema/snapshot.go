package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// SnapshotName is the schema snapshot file written next to generated code.
const SnapshotName = "ddb.snapshot.json"

// Snapshot is the on-disk schema snapshot format.
type Snapshot struct {
	FormatVersion int     `json:"format_version"`
	Schema        *Schema `json:"schema"`
}

// snapshotFormatVersion is bumped only when the snapshot JSON shape changes.
const snapshotFormatVersion = 1

// WriteSnapshot writes the schema snapshot with stable, sorted output.
func WriteSnapshot(path string, s *Schema) error {
	data, err := json.MarshalIndent(Snapshot{FormatVersion: snapshotFormatVersion, Schema: s}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ReadSnapshot loads a schema snapshot. It returns (nil, nil) when the file
// does not exist, which callers treat as "first generate".
func ReadSnapshot(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing snapshot %s: %w", path, err)
	}
	if snap.FormatVersion != snapshotFormatVersion {
		return nil, fmt.Errorf("snapshot %s has format_version %d; this ddbgen build supports %d",
			path, snap.FormatVersion, snapshotFormatVersion)
	}
	if snap.Schema == nil {
		// A schema-less snapshot must not silently disable the
		// breaking-change gate.
		return nil, fmt.Errorf("snapshot %s has no schema; delete it and regenerate", path)
	}
	return snap.Schema, nil
}
