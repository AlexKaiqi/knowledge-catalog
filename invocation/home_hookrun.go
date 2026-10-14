package invocation

import (
	"encoding/json"
	"strings"

	kchome "kc/home"
	"kc/hook"
	"kc/invoke"
	"kc/object/writer"
	"kc/snapshot"
)

func writerReplayed(ws *kchome.Home, flags map[string]invoke.FlagValue) bool {
	id := invoke.FlagString(flags, "command-id")
	if id == "" {
		return false
	}
	_, ok := ws.Commands.Lookup(id)
	return ok
}

func hookEvent(ws *kchome.Home, command string, flags map[string]invoke.FlagValue) hook.Event {
	return hook.Event{
		Action:    command,
		As:        invoke.FlagString(flags, "as"),
		Repo:      invoke.FlagString(flags, "repo"),
		SetID:     invoke.FlagString(flags, "dataset"),
		CommandID: invoke.FlagString(flags, "command-id"),
	}
}

func durableHome(ws *kchome.Home, home string) string {
	if ws != nil && strings.TrimSpace(ws.Dir) != "" {
		return ws.Dir
	}
	return home
}

func WithHooks(ws *kchome.Home, home, command string, flags map[string]invoke.FlagValue, observation *invoke.ObservationTelemetry, next func() (any, error)) (any, error) {
	observation = invoke.NoObservationTelemetry(observation)
	durable := durableHome(ws, home)
	defer observeHookOutbox(durable, observation)
	if !hook.CanHook(command) || writerReplayed(ws, flags) {
		return next()
	}
	event := hookEvent(ws, command, flags)
	if err := hook.PreObserved(durable, event, observation.Hook); err != nil {
		return nil, err
	}
	result, err := next()
	if err != nil {
		return nil, err
	}
	fillHookResult(&event, result)
	_ = hook.PostObserved(durable, event, observation.Hook)
	return result, nil
}

func observeHookOutbox(home string, observation *invoke.ObservationTelemetry) {
	if observation == nil || observation.HookBacklog == nil {
		return
	}
	stats, err := hook.InspectOutbox(home)
	if err != nil {
		return
	}
	observation.HookBacklog(stats.Pending, stats.OldestPendingAt)
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
	case snapshot.Receipt:
		event.Receipt = v.ReceiptRef
		event.Disposition = v.Disposition
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
