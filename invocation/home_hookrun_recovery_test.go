package invocation

import (
	"errors"
	"testing"

	"kc/home"
	"kc/invoke"
	"kc/snapshot"
)

func TestOnlyCompletedNoChangeIsHookReplay(t *testing.T) {
	ledger, err := snapshot.NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	staged := ledger.(snapshot.StagedCommandLedger)
	request := snapshot.CommandRequest{Kind: "COMMIT"}
	_, _, _ = staged.ExecuteStaged("no-change", "digest", request, func(attempt snapshot.CommandAttempt) (any, error) {
		if err := attempt.SelectNoChange(); err != nil {
			return nil, err
		}
		return nil, errors.New("receipt not saved")
	})
	ws := &home.Home{Commands: ledger}
	flags := map[string]invoke.FlagValue{"command-id": "no-change"}
	if writerReplayed(ws, flags) {
		t.Fatal("unfinished no-change recovery skipped fresh hooks")
	}
	_, _, err = staged.ExecuteStaged("no-change", "digest", request, func(snapshot.CommandAttempt) (any, error) { return map[string]any{"accepted": true}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !writerReplayed(ws, flags) {
		t.Fatal("durable no-change replay repeated hooks")
	}
	_, _, _ = ledger.Execute("unknown", "digest", request, func() (any, error) { return nil, errors.New("authority outcome unknown") })
	flags["command-id"] = "unknown"
	if !writerReplayed(ws, flags) {
		t.Fatal("ordinary unknown command reran hooks as a fresh write")
	}
}
