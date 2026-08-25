package cli

import (
	"encoding/json"

	"kc/hook"
	"kc/writer"
)

func writerReplayed(ws *Home, flags map[string]FlagValue) bool {
	id := FlagString(flags, "command-id")
	if id == "" {
		return false
	}
	_, ok := ws.Writer.Lookup(id)
	return ok
}

func hookEvent(command string, flags map[string]FlagValue) hook.Event {
	return hook.Event{
		Cmd:         command,
		As:          FlagString(flags, "as"),
		Repo:        FlagString(flags, "repo"),
		Catalog:     FlagString(flags, "catalog"),
		WorkspaceID: FlagString(flags, "workspace"),
		CommandID:   FlagString(flags, "command-id"),
	}
}

func withHooks(ws *Home, home, command string, flags map[string]FlagValue, next func() (any, error)) (any, error) {
	if !hook.CanHook(command) || writerReplayed(ws, flags) {
		return next()
	}
	event := hookEvent(command, flags)
	if event.Catalog == "" && len(ws.File.Catalogs) > 0 && catalogScoped(command) {
		event.Catalog = ws.File.Catalogs[0].ID
	}
	if err := hook.Pre(home, event); err != nil {
		return nil, err
	}
	result, err := next()
	if err != nil {
		return nil, err
	}
	fillHookResult(&event, result)
	_ = hook.Post(home, event)
	return result, nil
}

func catalogScoped(command string) bool {
	switch command {
	case "define-workspace", "preview", "validate", "record-validation",
		"retire-workspace", "register", "archive-catalog":
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
