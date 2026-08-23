package repository

import "kc/kernel"

// APPEND is layer ⓪ (ordered log). JSONL beside a git-shaped Snapshot is
// packing, not "stream is a git repo". Do not repo-add a stream. The reference
// implementation's producer ordering profile is NONE: cursor CAS orders the
// stored segment, while source-specific partition/position checkpoints remain
// connector concerns and are deliberately not fields on AppendEntry.

type AppendEntry struct {
	EventID    string `json:"eventId"`
	EventType  string `json:"eventType,omitempty"`
	Payload    any    `json:"payload"`
	ObservedAt string `json:"observedAt,omitempty"`
	SchemaRef  string `json:"schemaRef,omitempty"`
}

type AppendEntries struct {
	TargetRepository kernel.RepositoryID `json:"targetRepository"`
	StreamRef        string              `json:"streamRef"`
	ExpectedCursor   string              `json:"expectedCursor,omitempty"`
	Entries          []AppendEntry       `json:"entries"`
}

type StreamRecord struct {
	RecordID   string `json:"recordId"`
	EventID    string `json:"eventId"`
	Payload    any    `json:"payload"`
	Digest     string `json:"digest"`
	RecordedAt string `json:"recordedAt"`
	SchemaRef  string `json:"schemaRef,omitempty"`
}

type StreamSlice struct {
	Repository kernel.RepositoryID `json:"repository"`
	StreamRef  string              `json:"streamRef"`
	Cursor     string              `json:"cursor"`
	Records    []StreamRecord      `json:"records"`
}
