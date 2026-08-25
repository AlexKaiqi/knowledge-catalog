package observability

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kc/internal/jsonfile"
	"kc/kernel"
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

func (s *FileStore) Access(query AccessQuery) ([]AccessEvent, error) {
	all, err := readJSONL[AccessEvent](s.AccessPath)
	if err != nil {
		return nil, err
	}
	out := make([]AccessEvent, 0, len(all))
	for _, event := range all {
		if query.Principal != "" && event.Identity.Principal != query.Principal {
			continue
		}
		if query.OnBehalfOf != "" && event.Identity.OnBehalfOf != query.OnBehalfOf {
			continue
		}
		if query.Action != "" && event.Action != query.Action {
			continue
		}
		if query.TraceID != "" && event.Trace.TraceID != query.TraceID {
			continue
		}
		if (query.Repository != "" || query.Object != "") && !matchesKnowledge(event, query) {
			continue
		}
		out = append(out, event)
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[len(out)-query.Limit:]
	}
	return out, nil
}

func matchesKnowledge(event AccessEvent, query AccessQuery) bool {
	for _, target := range event.Knowledge {
		ref := target.KnowledgeRef
		if query.Repository != "" && ref.Repository != query.Repository {
			continue
		}
		if query.Object != "" && ref.Object != query.Object {
			continue
		}
		return true
	}
	return false
}

func (s *FileStore) Trace(traceID string) (TraceView, error) {
	access, err := s.Access(AccessQuery{TraceID: traceID})
	if err != nil {
		return TraceView{}, err
	}
	feedback, err := readJSONL[FeedbackEvent](s.FeedbackPath)
	if err != nil {
		return TraceView{}, err
	}
	view := TraceView{TraceID: traceID, Entries: []TraceEntry{}}
	for i := range access {
		event := access[i]
		view.Entries = append(view.Entries, TraceEntry{Kind: "access", OccurredAt: event.OccurredAt, Access: &event})
	}
	for i := range feedback {
		event := feedback[i]
		if event.Trace.TraceID != traceID {
			continue
		}
		view.Entries = append(view.Entries, TraceEntry{Kind: "feedback", OccurredAt: event.OccurredAt, Feedback: &event})
	}
	sort.SliceStable(view.Entries, func(i, j int) bool {
		return occurredBefore(view.Entries[i].OccurredAt, view.Entries[j].OccurredAt)
	})
	return view, nil
}

func occurredBefore(left, right string) bool {
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	if leftErr == nil && rightErr == nil {
		return leftTime.Before(rightTime)
	}
	return left < right
}

func (s *FileStore) Hitmap(query AccessQuery) ([]KnowledgeHit, error) {
	events, err := s.Access(query)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*KnowledgeHit{}
	for _, event := range events {
		if event.Decision != "ALLOW" || event.Result != "RESOLVED" {
			continue
		}
		seen := map[string]bool{}
		for _, target := range event.Knowledge {
			ref := target.KnowledgeRef
			key := strings.Join([]string{string(ref.Repository), string(ref.Commit), string(ref.Object), addressKey(target.Address)}, "\x00")
			if seen[key] {
				continue
			}
			seen[key] = true
			hit := byKey[key]
			if hit == nil {
				hit = &KnowledgeHit{
					KnowledgeRef: ref, Address: target.Address,
					FirstAccessedAt: event.OccurredAt, LastAccessedAt: event.OccurredAt,
					Principals: map[string]int{}, OnBehalfOf: map[string]int{},
				}
				byKey[key] = hit
			}
			hit.Hits++
			if occurredBefore(event.OccurredAt, hit.FirstAccessedAt) {
				hit.FirstAccessedAt = event.OccurredAt
			}
			if occurredBefore(hit.LastAccessedAt, event.OccurredAt) {
				hit.LastAccessedAt = event.OccurredAt
			}
			hit.Principals[event.Identity.Principal]++
			if event.Identity.OnBehalfOf != "" {
				hit.OnBehalfOf[event.Identity.OnBehalfOf]++
			}
		}
	}
	out := make([]KnowledgeHit, 0, len(byKey))
	for _, hit := range byKey {
		if len(hit.OnBehalfOf) == 0 {
			hit.OnBehalfOf = nil
		}
		out = append(out, *hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		a, b := out[i].KnowledgeRef, out[j].KnowledgeRef
		return string(a.Repository)+string(a.Object)+string(a.Commit) < string(b.Repository)+string(b.Object)+string(b.Commit)
	})
	return out, nil
}

func addressKey(address *kernel.Address) string {
	if address == nil {
		return ""
	}
	return kernel.AddressKey(*address)
}

func readJSONL[T any](path string) ([]T, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []T{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := []T{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var value T
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, scanner.Err()
}
