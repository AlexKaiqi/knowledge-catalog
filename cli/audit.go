package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"kc/catalog"
	"kc/internal/journal"
	"kc/knowledge/writer"
)

func auditPath(home string) string  { return filepath.Join(home, "audit.jsonl") }
func systemPath(home string) string { return filepath.Join(home, "system.jsonl") }

func shouldAudit(command string, flags map[string]FlagValue) bool {
	if FlagBool(flags, "help") {
		return false
	}
	switch command {
	case "", "help", "--help", "-h", "audit":
		return false
	}
	return true
}

func recordAudit(home, command string, flags map[string]FlagValue, result any, err error) error {
	if !shouldAudit(command, flags) {
		return nil
	}
	identity, identityErr := identityContextFrom(flags)
	if identityErr != nil {
		return identityErr
	}
	trace, traceErr := traceContextFrom(flags)
	if traceErr != nil {
		return traceErr
	}
	reqID, _ := requestIDFrom(flags)
	event := journal.Event{
		Layer:        journal.LayerKC,
		Face:         "cli",
		Cmd:          command,
		Principal:    identity.Principal,
		As:           identity.Principal,
		OnBehalfOf:   identity.OnBehalfOf,
		RequestID:    reqID,
		TraceID:      trace.TraceID,
		SpanID:       trace.SpanID,
		ParentSpanID: trace.ParentSpanID,
		SessionID:    trace.SessionID,
		RuleID:       matchedRuleID(home, command, flags),
		Args:         auditArgs(flags),
		Refs:         auditRefs(command, flags, result),
	}
	if err != nil {
		event.Status = "error"
		event.Error = journal.ErrorOf(err)
		event.Refs = auditRefs(command, flags, nil)
	}
	if recErr := journal.Record(journal.NewFile(auditPath(home)), event); recErr != nil {
		return recErr
	}
	return nil
}

func readTrail(home, layer, cmd string, limit int) ([]journal.Event, error) {
	var sets [][]journal.Event
	if layer == "" || layer == journal.LayerKC {
		kcEvents, err := journal.Read(auditPath(home))
		if err != nil {
			return nil, err
		}
		sets = append(sets, kcEvents)
	}
	if layer == "" || layer == journal.LayerSystem {
		sysEvents, err := journal.Read(systemPath(home))
		if err != nil {
			return nil, err
		}
		sets = append(sets, sysEvents)
	}
	return journal.Filter(journal.Merge(sets...), layer, cmd, limit), nil
}

func catalogLogEntries(commits []catalog.CatalogCommit, cmd string, limit int) []map[string]any {
	out := make([]map[string]any, 0, len(commits))
	for _, c := range commits {
		if cmd != "" {
			verb, _, _ := strings.Cut(c.Message, " ")
			if verb != cmd && c.Message != cmd {
				continue
			}
		}
		row := map[string]any{"commit": c.Commit, "message": c.Message}
		if c.Author != "" {
			row["author"] = c.Author
		}
		if c.RequestID != "" {
			row["requestId"] = c.RequestID
		}
		if c.RuleID != "" {
			row["ruleId"] = c.RuleID
		}
		out = append(out, row)
	}
	if limit > 0 && limit < 1<<30 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

var redactedFlag = map[string]bool{
	"value":   true,
	"payload": true,
	"message": true,
}

func auditArgs(flags map[string]FlagValue) map[string]any {
	if len(flags) == 0 {
		return nil
	}
	out := map[string]any{}
	for name, value := range flags {
		if name == "home" || strings.HasPrefix(name, "_") {
			continue
		}
		if redactedFlag[name] {
			out[name] = "<redacted>"
			continue
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func auditOmitResult(command string) bool {
	switch command {
	case "read", "list", "status", "provenance", "diff", "log", "allowed", "whoami", "receipt", "ingest", "resolve", "resolve-binding", "checkout",
		"access-log", "trace", "hitmap":
		return true
	}
	return false
}

func auditRefs(command string, flags map[string]FlagValue, result any) map[string]any {
	refs := map[string]any{}
	for _, name := range []string{
		"repo", "catalog", "object", "command-id", "workspace",
		"proposal", "preview", "proposal-id", "aspect", "trace-id", "session-id",
	} {
		if v := FlagString(flags, name); v != "" {
			refs[name] = v
		}
	}
	if result != nil && !auditOmitResult(command) {
		switch v := result.(type) {
		case writer.CommitReceipt:
			refs["disposition"] = string(v.Disposition)
			refs["commandId"] = v.CommandID
			refs["repositoryId"] = string(v.Result.RepositoryID)
			refs["newCommit"] = string(v.Result.NewCommit)
		default:
			raw := asRefMap(result)
			for _, k := range []string{
				"catalog", "repositoryId", "head", "workspaceId", "commitId",
				"id", "disposition", "commandId", "proposalId", "previewId", "reportId",
				"workspace", "retired", "archived",
			} {
				if val, ok := raw[k]; ok && val != nil && val != "" && val != false {
					refs[k] = val
				}
			}
			if res, ok := raw["result"].(map[string]any); ok {
				for _, k := range []string{"newCommit", "oldCommit", "commitId", "cursor", "repositoryId", "targetRef", "streamRef"} {
					if val, ok := res[k]; ok && val != nil && val != "" {
						refs[k] = val
					}
				}
			}
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func asRefMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}
