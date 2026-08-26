package observability

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"kc/internal/jsonfile"
)

type FileStore struct {
	AccessPath   string
	FeedbackPath string
}

func NewFileStore(home string) *FileStore {
	return &FileStore{
		AccessPath:   filepath.Join(home, "access.jsonl"),
		FeedbackPath: filepath.Join(home, "feedback.jsonl"),
	}
}

func (s *FileStore) RecordAccess(event AccessEvent) error {
	_, err := s.RecordAccessReceipt(event)
	return err
}

// RecordAccessReceipt durably appends access evidence and returns the stable
// identifier written into the record. A caller can place that identifier in
// its audit acknowledgement without parsing the JSONL file back.
func (s *FileStore) RecordAccessReceipt(event AccessEvent) (string, error) {
	if event.EvidenceID != "" {
		return "", fmt.Errorf("evidenceId is recorder-managed")
	}
	id, err := newEvidenceID()
	if err != nil {
		return "", err
	}
	event.EvidenceID = id
	if err := event.Validate(); err != nil {
		return "", err
	}
	if event.OccurredAt == "" {
		event.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Knowledge == nil {
		event.Knowledge = []KnowledgeAccess{}
	}
	if event.Files == nil {
		event.Files = []FileAccess{}
	}
	if event.Snapshots == nil {
		event.Snapshots = []SnapshotAccess{}
	}
	if err := jsonfile.AppendJSONL(s.AccessPath, event); err != nil {
		return "", err
	}
	return event.EvidenceID, nil
}

func newEvidenceID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate evidence id: %w", err)
	}
	return "ev_" + hex.EncodeToString(raw), nil
}

func (s *FileStore) RecordFeedback(event FeedbackEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.OccurredAt == "" {
		event.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return jsonfile.AppendJSONL(s.FeedbackPath, event)
}
