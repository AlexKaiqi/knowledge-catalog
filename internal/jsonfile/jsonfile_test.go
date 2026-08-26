package jsonfile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteReadAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := Write(path, map[string]any{"revision": 1}); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, map[string]any{"revision": 2}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Revision int `json:"revision"`
	}
	if err := Read(path, &got); err != nil || got.Revision != 2 {
		t.Fatalf("read replaced file: %#v %v", got, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("atomic write left a temporary file: %v", err)
	}
}

func TestAppendJSONLConcurrentRecordsStayWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	const count = 32
	var wait sync.WaitGroup
	for id := 0; id < count; id++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := AppendJSONL(path, map[string]any{"id": id}); err != nil {
				t.Errorf("append %d: %v", id, err)
			}
		}()
	}
	wait.Wait()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
	if len(lines) != count {
		t.Fatalf("got %d records, want %d: %q", len(lines), count, body)
	}
	seen := map[int]bool{}
	for _, line := range lines {
		var row struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatalf("torn JSONL record %q: %v", line, err)
		}
		seen[row.ID] = true
	}
	if len(seen) != count {
		t.Fatalf("records were lost or duplicated: %#v", seen)
	}
}

func TestAppendJSONLWritesOneObjectPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events", "access.jsonl")
	if err := AppendJSONL(path, map[string]any{"id": 1}); err != nil {
		t.Fatal(err)
	}
	if err := AppendJSONL(path, map[string]any{"id": 2}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
	if len(lines) != 2 || !bytes.Contains(lines[0], []byte(`"id":1`)) || !bytes.Contains(lines[1], []byte(`"id":2`)) {
		t.Fatalf("unexpected JSONL: %q", body)
	}
}
