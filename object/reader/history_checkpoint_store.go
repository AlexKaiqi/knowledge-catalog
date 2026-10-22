package reader

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"kc/kernel"
	objectmodel "kc/object"
	"strings"
)

var historyCheckpointBucket = []byte("object-history-checkpoints-v1")

// HistoryCheckpointStore retains server-issued, immutable scan progress across
// process restarts. A reference never grants permission or retains an index.
type HistoryCheckpointStore interface {
	Retain(context.Context, objectmodel.HistoryCheckpoint) (string, error)
	Resolve(context.Context, string) (objectmodel.HistoryCheckpoint, error)
}

func NewHistoryCheckpointStore(path string) (HistoryCheckpointStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "history checkpoint storage requires a durable state path")
	}
	return &historyCheckpointStore{records: coordinateRecords{path: path, bucket: historyCheckpointBucket}}, nil
}

type historyCheckpointStore struct{ records coordinateRecords }

func (s *historyCheckpointStore) Retain(ctx context.Context, checkpoint objectmodel.HistoryCheckpoint) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := checkpoint.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return "", err
	}
	reference := historyCheckpointReference(checkpoint)
	err = s.records.put(ctx, reference, raw, func(raw []byte) error { _, err := decodeHistoryCheckpoint(raw, reference); return err })
	if err != nil {
		return "", err
	}
	return reference, nil
}

func (s *historyCheckpointStore) Resolve(ctx context.Context, reference string) (objectmodel.HistoryCheckpoint, error) {
	if err := ctx.Err(); err != nil {
		return objectmodel.HistoryCheckpoint{}, err
	}
	if !strings.HasPrefix(reference, "hs1_") || len(reference) != 68 {
		return objectmodel.HistoryCheckpoint{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid history continuation reference")
	}
	if _, err := hex.DecodeString(reference[4:]); err != nil {
		return objectmodel.HistoryCheckpoint{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid history continuation reference")
	}
	raw, err := s.records.get(ctx, reference)
	if err != nil {
		return objectmodel.HistoryCheckpoint{}, err
	}
	return decodeHistoryCheckpoint(raw, reference)
}

func historyCheckpointReference(checkpoint objectmodel.HistoryCheckpoint) string {
	return "hs1_" + string(kernel.CanonicalDigest(checkpoint))
}

func decodeHistoryCheckpoint(raw []byte, reference string) (objectmodel.HistoryCheckpoint, error) {
	var checkpoint objectmodel.HistoryCheckpoint
	if err := json.Unmarshal(raw, &checkpoint); err != nil || checkpoint.Validate() != nil || historyCheckpointReference(checkpoint) != reference {
		return objectmodel.HistoryCheckpoint{}, kernel.Fail(kernel.ErrPreconditionFailed, "durable history checkpoint is corrupt")
	}
	return checkpoint, nil
}
