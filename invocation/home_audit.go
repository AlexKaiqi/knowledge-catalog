package invocation

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"kc/internal/journal"
	"kc/invoke"
	"kc/object/writer"
	"kc/snapshot/dataset"
)

func auditPath(home string) string  { return filepath.Join(home, "audit.jsonl") }
func systemPath(home string) string { return filepath.Join(home, "system.jsonl") }

func shouldAudit(command string, flags map[string]invoke.FlagValue) bool {
	if invoke.FlagBool(flags, "help") {
		return false
	}
	switch command {
	case "", "help", "--help", "-h", "audit":
		return false
	}
	return true
}

func RecordAudit(home, command string, flags map[string]invoke.FlagValue, result any, err error) (int64, error) {
	if !shouldAudit(command, flags) {
		return -1, nil
	}
	identity, identityErr := IdentityContextFrom(flags)
	if identityErr != nil {
		return -1, identityErr
	}
	trace, traceErr := TraceContextFrom(flags)
	if traceErr != nil {
		return -1, traceErr
	}
	reqID, _ := RequestIDFrom(flags)
	event := journal.Event{
		Layer:        journal.LayerKC,
		Face:         "cli",
		Cmd:          ActionOf(command, flags),
		Principal:    identity.Principal,
		As:           identity.Principal,
		OnBehalfOf:   identity.OnBehalfOf,
		RequestID:    reqID,
		EvidenceID:   invoke.FlagString(flags, "_evidence-id"),
		TraceID:      trace.TraceID,
		SpanID:       trace.SpanID,
		ParentSpanID: trace.ParentSpanID,
		RuleID:       MatchedRuleID(home, command, flags),
		Args:         auditArgs(flags),
		Refs:         auditRefs(command, flags, result),
	}
	if err != nil {
		event.Status = "error"
		event.Error = journal.ErrorOf(err)
		event.Refs = auditRefs(command, flags, nil)
	}
	raw, _ := json.Marshal(event)
	bytes := int64(len(raw) + 1)
	if recErr := journal.Record(journal.NewFile(auditPath(home)), event); recErr != nil {
		return bytes, recErr
	}
	return bytes, nil
}

func ReadTrail(home, layer, cmd string, limit int) ([]journal.Event, error) {
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

func RegistryLogEntries(commits []dataset.Commit, cmd string, limit int) []map[string]any {
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
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

var redactedFlag = map[string]bool{
	"value":   true,
	"payload": true,
	"message": true,
}

func auditArgs(flags map[string]invoke.FlagValue) map[string]any {
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
	case "object-read", "list", "local-status", "object-provenance", "object-log", "admin-grant-list", "whoami",
		"writer-receipt", "pin", "object-resolve", "object-binding-show",
		"diff",
		"operations-audit-access", "operations-audit-trace", "operations-audit-hitmap":
		return true
	}
	return false
}

func auditRefs(command string, flags map[string]invoke.FlagValue, result any) map[string]any {
	refs := map[string]any{}
	for _, name := range []string{
		"repo", "object", "command-id", "dataset",
		"aspect", "trace-id", "implementation",
	} {
		if v := invoke.FlagString(flags, name); v != "" {
			refs[name] = v
		}
	}
	if result != nil && !auditOmitResult(command) {
		for key, value := range objectInterfaceAuditRefs(result) {
			refs[key] = value
		}
		switch v := result.(type) {
		case writer.CommitReceipt:
			refs["disposition"] = string(v.Disposition)
			refs["commandId"] = v.CommandID
			refs["repositoryId"] = string(v.Result.RepositoryID)
			refs["newCommit"] = string(v.Result.NewCommit)
		default:
			raw := asRefMap(result)
			for _, k := range []string{
				"repositoryId", "head", "setId", "commitId",
				"id", "disposition", "commandId",
				"dataset", "retired", "archived",
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
