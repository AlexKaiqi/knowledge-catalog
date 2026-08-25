package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"time"

	"kc/internal/jsonfile"
	"kc/kernel"
)

// Two durable trails, both pointers-only (not knowledge bodies):
//
//	layer=system  Writer / Catalog / ControlPlane / Reader
//	layer=kc      CLI facade (init, allow, argv, --as)
const (
	LayerSystem = "system"
	LayerKC     = "kc"
)

type Event struct {
	Time      string `json:"time"`
	Layer     string `json:"layer"`
	Face      string `json:"face,omitempty"`
	Cmd       string `json:"cmd"`
	Principal string `json:"principal,omitempty"`
	// As is retained for compatibility with older local trail readers.
	As           string         `json:"as,omitempty"`
	OnBehalfOf   string         `json:"onBehalfOf,omitempty"`
	RequestID    string         `json:"requestId,omitempty"`
	TraceID      string         `json:"traceId,omitempty"`
	SpanID       string         `json:"spanId,omitempty"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	SessionID    string         `json:"sessionId,omitempty"`
	RuleID       string         `json:"ruleId,omitempty"`
	Status       string         `json:"status"`
	Error        map[string]any `json:"error,omitempty"`
	Args         map[string]any `json:"args,omitempty"`
	Refs         map[string]any `json:"refs,omitempty"`
}

type Journal interface {
	Record(Event) error
}

type File struct {
	Path string
}

func NewFile(path string) *File {
	return &File{Path: path}
}

func (f *File) Record(event Event) error {
	if f == nil || f.Path == "" {
		return nil
	}
	if event.Time == "" {
		event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Status == "" {
		event.Status = "ok"
	}
	return jsonfile.AppendJSONL(f.Path, event)
}

func Record(j Journal, event Event) error {
	if j == nil {
		return nil
	}
	if event.Time == "" {
		event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Status == "" {
		if event.Error != nil {
			event.Status = "error"
		} else {
			event.Status = "ok"
		}
	}
	return j.Record(event)
}

// Finish writes one event. The original err wins if both fail.
func Finish(j Journal, layer, face, cmd string, refs map[string]any, err error) error {
	event := Event{Layer: layer, Face: face, Cmd: cmd, Status: "ok", Refs: refs}
	if err != nil {
		event.Status = "error"
		event.Error = ErrorOf(err)
	}
	recErr := Record(j, event)
	if err != nil {
		return err
	}
	return recErr
}

func ErrorOf(err error) map[string]any {
	if err == nil {
		return nil
	}
	if e := kernel.AsIngress(err); e != nil {
		return map[string]any{"code": string(e.Code), "message": e.Message}
	}
	return map[string]any{"message": err.Error()}
}

func Read(path string) ([]Event, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []Event{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if events == nil {
		return []Event{}, nil
	}
	return events, nil
}

func Merge(sets ...[]Event) []Event {
	var out []Event
	for _, set := range sets {
		out = append(out, set...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Time < out[j].Time
	})
	return out
}

func Filter(events []Event, layer, cmd string, limit int) []Event {
	var out []Event
	for _, event := range events {
		if layer != "" && event.Layer != layer {
			continue
		}
		if cmd != "" && event.Cmd != cmd {
			continue
		}
		out = append(out, event)
	}
	if out == nil {
		out = []Event{}
	}
	if limit <= 0 {
		limit = 50
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
