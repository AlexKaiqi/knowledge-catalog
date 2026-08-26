package commandlog

import (
	"fmt"
	"testing"
)

func TestCommandLocksAreReleased(t *testing.T) {
	ledger, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("command-%d", i)
		if _, _, err := ledger.Execute(id, id, Request{Kind: "TEST"}, func() (any, error) {
			return map[string]any{"ok": true}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.locks) != 0 {
		t.Fatalf("completed commands retained %d command locks", len(ledger.locks))
	}
}
