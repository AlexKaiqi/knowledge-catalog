package cli

import (
	"encoding/json"

	"kc/hook"
	"kc/knowledge/writer"
	"kc/snapshot/treewriter"
)

func writerReplayed(ws *Home, flags map[string]FlagValue) bool {
	id := FlagString(flags, "command-id")
	if id == "" {
		return false
	}
	_, ok := ws.Commands.Lookup(id)
	return ok
}

func hookEvent(command string, flags map[string]FlagValue) hook.Event {
	return hook.Event{
		Action:      command,
		As:          FlagString(flags, "as"),
		Repo:        FlagString(flags, "repo"),
		Catalog:     FlagString(flags, "catalog"),
		WorkspaceID: FlagString(flags, "workspace"),
		CommandID:   FlagString(flags, "command-id"),
	}
}

func withHooks(ws *Home, home, command string, flags map[string]FlagValue, observation *operationTelemetry, next func() (any, error)) (any, error) {
	observation = noOperationTelemetry(observation)
	defer observeHookOutbox(home, observation)
	if !hook.CanHook(command) || writerReplayed(ws, flags) {
		return next()
	}
	event := hookEvent(command, flags)
	if event.Catalog == "" && len(ws.File.Catalogs) > 0 && catalogScoped(command) {
		event.Catalog = ws.File.Catalogs[0].ID
	}
	if err := hook.PreObserved(home, event, observation.hook); err != nil {
		return nil, err
	}
	result, err := next()
	if err != nil {
		return nil, err
	}
	fillHookResult(&event, result)
	_ = hook.PostObserved(home, event, observation.hook)
	return result, nil
}

func observeHookOutbox(home string, observation *operationTelemetry) {
	if observation == nil || observation.hookBacklog == nil {
		return
	}
	stats, err := hook.InspectOutbox(home)
	if err != nil {
		return
	}
	observation.hookBacklog(stats.Pending, stats.OldestPendingAt)
}

func catalogScoped(command string) bool {
	switch command {
	case "workspace.manage", "governance.preview.create", "governance.validate",
		"governance.validation.record", "catalog.repositories.manage", "catalog.manage":
		return true
	default:
		return false
	}
}

func fillHookResult(event *hook.Event, result any) {
	if result == nil {
		return
	}
	switch v := result.(type) {
	case writer.CommitReceipt:
		event.Receipt = v.ReceiptRef
		event.Disposition = string(v.Disposition)
		event.NewCommit = string(v.Result.NewCommit)
		event.CommandID = v.CommandID
	case treewriter.Receipt:
		event.Receipt = v.ReceiptRef
		event.Disposition = string(v.Disposition)
		event.NewCommit = string(v.Result.NewCommit)
		event.CommandID = v.CommandID
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			return
		}
		if s, _ := m["workspaceId"].(string); s != "" {
			event.WorkspaceID = s
		}
		if s, _ := m["commitId"].(string); s != "" {
			event.NewCommit = s
		}
		if s, _ := m["receiptRef"].(string); s != "" {
			event.Receipt = s
		}
		if s, _ := m["disposition"].(string); s != "" {
			event.Disposition = s
		}
		if s, _ := m["newCommit"].(string); s != "" {
			event.NewCommit = s
		}
	}
}
