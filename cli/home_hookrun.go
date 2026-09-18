package cli

import (
	"encoding/json"
	"strings"

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

func hookEvent(ws *Home, command string, flags map[string]FlagValue) hook.Event {
	catalogID := FlagString(flags, "catalog")
	if catalogID == "" {
		catalogID = FlagString(flags, "_default-catalog")
	}
	if catalogID == "" && ws != nil && len(ws.File.Catalogs) == 1 {
		catalogID = ws.File.Catalogs[0].ID
	}
	return hook.Event{
		Action:      command,
		As:          FlagString(flags, "as"),
		Repo:        FlagString(flags, "repo"),
		Catalog:     catalogID,
		SetID: FlagString(flags, "dataset"),
		CommandID:   FlagString(flags, "command-id"),
	}
}

func durableHome(ws *Home, home string) string {
	if ws != nil && strings.TrimSpace(ws.Dir) != "" {
		return ws.Dir
	}
	return home
}

func withHooks(ws *Home, home, command string, flags map[string]FlagValue, observation *operationTelemetry, next func() (any, error)) (any, error) {
	observation = noOperationTelemetry(observation)
	durable := durableHome(ws, home)
	defer observeHookOutbox(durable, observation)
	if !hook.CanHook(command) || writerReplayed(ws, flags) {
		return next()
	}
	event := hookEvent(ws, command, flags)
	if err := hook.PreObserved(durable, event, observation.hook); err != nil {
		return nil, err
	}
	result, err := next()
	if err != nil {
		return nil, err
	}
	fillHookResult(&event, result)
	_ = hook.PostObserved(durable, event, observation.hook)
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
		if s, _ := m["setId"].(string); s != "" {
			event.SetID = s
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
