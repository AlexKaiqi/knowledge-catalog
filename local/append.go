package local

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kc/kernel"
	"kc/repository"
)

// JSONLStream is layer ⓪ Stream packed beside a git-shaped Snapshot.
// It is not a git object and not a Catalog member.

var _ repository.Stream = (*JSONLStream)(nil)

type JSONLStream struct {
	rootDir string
	bind    kernel.RepositoryID
}

func NewJSONLStream(rootDir string, bind kernel.RepositoryID) *JSONLStream {
	return &JSONLStream{rootDir: rootDir, bind: bind}
}

func streamPath(rootDir, streamRef string) string {
	return filepath.Join(rootDir, "streams", url.QueryEscape(streamRef)+".jsonl")
}

func (s *JSONLStream) Append(streamRef string, entries []repository.AppendEntry, expectedCursor string) ([]string, error) {
	file := streamPath(s.rootDir, streamRef)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	existing := s.loadStreamRecords(streamRef)
	byEvent := map[string]repository.StreamRecord{}
	for _, rec := range existing {
		byEvent[rec.EventID] = rec
	}
	count := len(existing)
	if expectedCursor != "" && expectedCursor != strconv.Itoa(count) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "expected stream cursor %s but cursor is %d", expectedCursor, count)
	}
	var appended []string
	var newLines []string
	for _, entry := range entries {
		digest := string(kernel.CanonicalDigest(entry.Payload))
		if prior, ok := byEvent[entry.EventID]; ok {
			if prior.Digest != digest {
				return nil, kernel.Fail(kernel.ErrEventIDConflict, "event id %s already used with different payload", entry.EventID)
			}
			appended = append(appended, prior.RecordID)
			continue
		}
		observed := entry.ObservedAt
		if observed == "" {
			observed = time.Now().UTC().Format(time.RFC3339Nano)
		}
		record := repository.StreamRecord{
			RecordID:   "rec-" + strconv.Itoa(count+len(newLines)+1),
			EventID:    entry.EventID,
			Payload:    entry.Payload,
			Digest:     digest,
			RecordedAt: observed,
			SchemaRef:  entry.SchemaRef,
		}
		b, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		newLines = append(newLines, string(b))
		byEvent[entry.EventID] = record
		appended = append(appended, record.RecordID)
	}
	if len(newLines) > 0 {
		f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		_, err = f.WriteString(strings.Join(newLines, "\n") + "\n")
		_ = f.Close()
		if err != nil {
			return nil, err
		}
	}
	return appended, nil
}

func (s *JSONLStream) StreamCursor(streamRef string) string {
	return strconv.Itoa(len(s.loadStreamRecords(streamRef)))
}

func (s *JSONLStream) ReadStream(streamRef string) repository.StreamSlice {
	records := s.loadStreamRecords(streamRef)
	return repository.StreamSlice{Repository: s.bind, StreamRef: streamRef, Cursor: strconv.Itoa(len(records)), Records: records}
}

func (s *JSONLStream) loadStreamRecords(streamRef string) []repository.StreamRecord {
	file := streamPath(s.rootDir, streamRef)
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var records []repository.StreamRecord
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec repository.StreamRecord
		if json.Unmarshal([]byte(line), &rec) == nil {
			records = append(records, rec)
		}
	}
	return records
}
