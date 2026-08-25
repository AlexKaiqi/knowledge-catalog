package observability

import (
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
	if err := event.Validate(); err != nil {
		return err
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
	return jsonfile.AppendJSONL(s.AccessPath, event)
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
