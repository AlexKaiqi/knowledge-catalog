package gitdir

import (
	"strconv"
	"strings"
)

const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

type LogEntry struct {
	Commit    string `json:"commit"`
	Author    string `json:"author,omitempty"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	RuleID    string `json:"ruleId,omitempty"`
}

// Log reads commit history, newest first, optionally limited to one path.
func (d *Dir) Log(limit int, path string) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	args := []string{"log", "-" + strconv.Itoa(limit), "--format=%H" + fieldSep + "%an" + fieldSep + "%s" + fieldSep + "%b" + recordSep}
	if path != "" {
		args = append(args, "--", path)
	}
	raw, err := d.Git(args...)
	if err != nil {
		return []LogEntry{}, nil
	}
	entries := []LogEntry{}
	for _, record := range strings.Split(strings.Trim(raw, recordSep+"\n "), recordSep) {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, fieldSep, 4)
		if len(parts) < 3 {
			continue
		}
		entry := LogEntry{Commit: parts[0], Author: parts[1], Message: parts[2]}
		if len(parts) > 3 {
			entry.RequestID, entry.RuleID = ParseTrailers(parts[3])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
