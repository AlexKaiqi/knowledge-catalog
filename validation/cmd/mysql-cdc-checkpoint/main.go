package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const checkpointVersion = 1

type binlogEvent struct {
	SourceRef  string         `json:"sourceRef"`
	EventID    string         `json:"eventId"`
	BinlogFile string         `json:"binlogFile"`
	Position   uint64         `json:"position"`
	Operation  string         `json:"operation"`
	Schema     string         `json:"schema"`
	Table      string         `json:"table"`
	Key        map[string]any `json:"key"`
	Before     map[string]any `json:"before"`
	After      map[string]any `json:"after"`
}

type checkpoint struct {
	Version       int    `json:"version"`
	BinlogFile    string `json:"binlogFile"`
	Position      uint64 `json:"position"`
	EventID       string `json:"eventId"`
	PayloadDigest string `json:"payloadDigest"`
	StreamCursor  string `json:"streamCursor"`
	CatalogCommit string `json:"catalogCommit"`
}

type decision struct {
	Decision      string `json:"decision"`
	PayloadDigest string `json:"payloadDigest"`
	BinlogFile    string `json:"binlogFile"`
	Position      uint64 `json:"position"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	var eventPath, checkpointPath, streamCursor, catalogCommit string
	flag.StringVar(&eventPath, "event", "", "MySQL row-binlog event JSON")
	flag.StringVar(&checkpointPath, "checkpoint", "", "connector-owned checkpoint JSON")
	flag.StringVar(&streamCursor, "advance-stream-cursor", "", "persist an accepted event at this stream cursor")
	flag.StringVar(&catalogCommit, "catalog-commit", "", "catalog commit produced from the event")
	flag.Parse()
	if eventPath == "" || checkpointPath == "" {
		fatal("USAGE_INVALID", "--event and --checkpoint are required")
	}

	event, digest, err := readEvent(eventPath)
	if err != nil {
		fatal("EVENT_INVALID", err.Error())
	}
	prior, exists, err := readCheckpoint(checkpointPath)
	if err != nil {
		fatal("CHECKPOINT_INVALID", err.Error())
	}
	action, err := classify(event, digest, prior, exists)
	if err != nil {
		fatal("POSITION_REGRESSION", err.Error())
	}
	if streamCursor == "" {
		writeJSON(os.Stdout, decision{Decision: action, PayloadDigest: digest, BinlogFile: event.BinlogFile, Position: event.Position})
		return
	}
	if action != "APPLY" {
		fatal("CHECKPOINT_NOT_ADVANCED", "only an APPLY decision can advance the checkpoint")
	}
	if catalogCommit == "" {
		fatal("USAGE_INVALID", "--catalog-commit is required when advancing")
	}
	next := checkpoint{
		Version: checkpointVersion, BinlogFile: event.BinlogFile, Position: event.Position,
		EventID: event.EventID, PayloadDigest: digest, StreamCursor: streamCursor, CatalogCommit: catalogCommit,
	}
	if err := writeCheckpoint(checkpointPath, next); err != nil {
		fatal("CHECKPOINT_WRITE_FAILED", err.Error())
	}
	writeJSON(os.Stdout, next)
}

func readEvent(path string) (binlogEvent, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return binlogEvent{}, "", err
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return binlogEvent{}, "", err
	}
	canonical, err := json.Marshal(raw)
	if err != nil {
		return binlogEvent{}, "", err
	}
	sum := sha256.Sum256(canonical)
	var event binlogEvent
	if err := json.Unmarshal(canonical, &event); err != nil {
		return binlogEvent{}, "", err
	}
	if event.SourceRef == "" || event.EventID == "" || event.BinlogFile == "" || event.Position == 0 || event.Operation == "" || event.Schema == "" || event.Table == "" {
		return binlogEvent{}, "", errors.New("sourceRef, eventId, binlogFile, positive position, operation, schema, and table are required")
	}
	return event, hex.EncodeToString(sum[:]), nil
}

func readCheckpoint(path string) (checkpoint, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return checkpoint{}, false, nil
	}
	if err != nil {
		return checkpoint{}, false, err
	}
	var value checkpoint
	if err := json.Unmarshal(body, &value); err != nil {
		return checkpoint{}, false, err
	}
	if value.Version != checkpointVersion || value.BinlogFile == "" || value.Position == 0 || value.EventID == "" || value.PayloadDigest == "" || value.StreamCursor == "" || value.CatalogCommit == "" {
		return checkpoint{}, false, errors.New("checkpoint must contain version 1 and all cursor fields")
	}
	return value, true, nil
}

func classify(event binlogEvent, digest string, prior checkpoint, exists bool) (string, error) {
	if !exists {
		return "APPLY", nil
	}
	comparison, err := comparePosition(event.BinlogFile, event.Position, prior.BinlogFile, prior.Position)
	if err != nil {
		return "", err
	}
	if comparison < 0 {
		return "", fmt.Errorf("event %s:%d precedes checkpoint %s:%d", event.BinlogFile, event.Position, prior.BinlogFile, prior.Position)
	}
	if comparison == 0 {
		if event.EventID == prior.EventID && digest == prior.PayloadDigest {
			return "REPLAY", nil
		}
		return "", fmt.Errorf("event at checkpoint %s:%d does not match accepted event %s", event.BinlogFile, event.Position, prior.EventID)
	}
	return "APPLY", nil
}

func comparePosition(leftFile string, leftPos uint64, rightFile string, rightPos uint64) (int, error) {
	leftPrefix, leftSeq, err := splitBinlogFile(leftFile)
	if err != nil {
		return 0, err
	}
	rightPrefix, rightSeq, err := splitBinlogFile(rightFile)
	if err != nil {
		return 0, err
	}
	if leftPrefix != rightPrefix {
		return 0, fmt.Errorf("binlog prefixes differ: %s and %s", leftPrefix, rightPrefix)
	}
	if leftSeq < rightSeq || leftSeq == rightSeq && leftPos < rightPos {
		return -1, nil
	}
	if leftSeq == rightSeq && leftPos == rightPos {
		return 0, nil
	}
	return 1, nil
}

func splitBinlogFile(value string) (string, uint64, error) {
	index := strings.LastIndexByte(value, '.')
	if index <= 0 || index == len(value)-1 {
		return "", 0, fmt.Errorf("invalid binlog file %q", value)
	}
	sequence, err := strconv.ParseUint(value[index+1:], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid binlog file %q", value)
	}
	return value[:index], sequence, nil
}

func writeCheckpoint(path string, value checkpoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func writeJSON(file *os.File, value any) {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		os.Exit(1)
	}
}

func fatal(code, message string) {
	var envelope errorEnvelope
	envelope.Error.Code = code
	envelope.Error.Message = message
	writeJSON(os.Stderr, envelope)
	os.Exit(1)
}
