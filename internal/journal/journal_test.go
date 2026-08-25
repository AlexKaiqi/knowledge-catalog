package journal_test

import (
	"path/filepath"
	"testing"

	"kc/internal/journal"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestFileRoundTrip(t *testing.T) {
	path := filepath.Join(testkit.TempDir(t), "system.jsonl")
	j := journal.NewFile(path)
	if err := journal.Finish(j, journal.LayerSystem, "writer", "COMMIT", map[string]any{"repositoryId": "kr://acme/core"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := journal.Finish(j, journal.LayerSystem, "writer", "COMMIT", nil, kernel.Fail(kernel.ErrForbidden, "no")); err == nil {
		t.Fatal("want original error")
	}
	events, err := journal.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Layer != journal.LayerSystem || events[0].Cmd != "COMMIT" || events[1].Status != "error" {
		t.Fatal(events)
	}
	if journal.Finish(nil, journal.LayerSystem, "writer", "COMMIT", nil, nil) != nil {
		t.Fatal("nil journal must be a no-op")
	}
}

type memJournal struct{ events []journal.Event }

func (m *memJournal) Record(event journal.Event) error {
	m.events = append(m.events, event)
	return nil
}

func TestMultiRecordsEachSink(t *testing.T) {
	a, b := &memJournal{}, &memJournal{}
	j := journal.NewMulti(a, b)
	if err := journal.Finish(j, journal.LayerSystem, "writer", "COMMIT", map[string]any{"n": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if len(a.events) != 1 || len(b.events) != 1 || a.events[0].Cmd != "COMMIT" || b.events[0].Face != "writer" {
		t.Fatal(a.events, b.events)
	}
	if journal.NewMulti(nil, nil) != nil {
		t.Fatal("all-nil multi")
	}
}

func TestStampFillsEmptyIdentity(t *testing.T) {
	mem := &memJournal{}
	j := journal.WithStamp(mem, "agent:payments", "req-1", "alw_1")
	if err := journal.Finish(j, journal.LayerSystem, "writer", "COMMIT", map[string]any{"repositoryId": "kr://acme/core"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(mem.events) != 1 {
		t.Fatal(mem.events)
	}
	got := mem.events[0]
	if got.As != "agent:payments" || got.RequestID != "req-1" || got.RuleID != "alw_1" {
		t.Fatal(got)
	}
	if err := mem.Record(journal.Event{Cmd: "put", As: "other", RequestID: "req-2"}); err != nil {
		t.Fatal(err)
	}
	if mem.events[1].As != "other" || mem.events[1].RequestID != "req-2" {
		t.Fatal(mem.events[1])
	}
}

func TestContextStampPreservesDelegationAndTrace(t *testing.T) {
	mem := &memJournal{}
	j := journal.WithContext(mem, journal.Stamp{
		Principal: "agent:finance", OnBehalfOf: "user:kai", RequestID: "req-1",
		TraceID: "trace-1", SpanID: "span-1", ParentSpanID: "root", SessionID: "session-1", RuleID: "alw_1",
	})
	if err := journal.Finish(j, journal.LayerSystem, "reader", "read", map[string]any{"object": "Metric:gmv"}, nil); err != nil {
		t.Fatal(err)
	}
	got := mem.events[0]
	if got.Principal != "agent:finance" || got.As != "agent:finance" || got.OnBehalfOf != "user:kai" ||
		got.TraceID != "trace-1" || got.SpanID != "span-1" || got.ParentSpanID != "root" || got.SessionID != "session-1" {
		t.Fatal(got)
	}
}
