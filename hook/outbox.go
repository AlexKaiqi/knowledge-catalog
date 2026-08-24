package hook

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"kc/internal/jsonfile"
)

type OutboxItem struct {
	Binding Binding `json:"binding"`
	Event   Event   `json:"event"`
	Error   string  `json:"error,omitempty"`
}

func OutboxPath(home string) string { return filepath.Join(home, "hook-outbox.jsonl") }

func AppendOutbox(home string, b Binding, event Event, deliverErr error) error {
	item := OutboxItem{Binding: b, Event: event}
	if deliverErr != nil {
		item.Error = deliverErr.Error()
	}
	return jsonfile.AppendJSONL(OutboxPath(home), item)
}

func FlushOutbox(home string) error {
	path := OutboxPath(home)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	// Decode line by line via json.Decoder on the whole file.
	remaining := []OutboxItem{}
	dec := json.NewDecoder(bytes.NewReader(body))
	for {
		var item OutboxItem
		if err := dec.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if err := deliver(home, item.Binding, item.Event); err != nil {
			item.Error = err.Error()
			remaining = append(remaining, item)
		}
	}
	_ = os.Remove(path)
	for _, item := range remaining {
		if err := jsonfile.AppendJSONL(path, item); err != nil {
			return err
		}
	}
	return nil
}
