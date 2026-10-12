package snapshot

import (
	"fmt"
	"testing"
)

func TestCommandLocksAreReleased(t *testing.T) {
	ledger, err := NewCommandLedger(nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("command-%d", i)
		if _, _, err := ledger.Execute(id, id, CommandRequest{Kind: "TEST"}, func() (any, error) {
			return map[string]any{"ok": true}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	ledger.(*commandLedger).mu.Lock()
	defer ledger.(*commandLedger).mu.Unlock()
	if len(ledger.(*commandLedger).locks) != 0 {
		t.Fatalf("completed commands retained %d command locks", len(ledger.(*commandLedger).locks))
	}
}
