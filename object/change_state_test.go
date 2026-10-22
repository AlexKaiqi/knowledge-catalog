package object_test

import (
	"encoding/json"
	"strings"
	"testing"

	objectmodel "kc/object"
)

func TestChangeStateKeepsOptionalBooleanWireAndComparableReceiptValues(t *testing.T) {
	for _, row := range []struct {
		state objectmodel.ChangeState
		field string
	}{{objectmodel.ChangeUnknown, ""}, {objectmodel.ChangeUnchanged, `"changed":false`}, {objectmodel.ChangeChanged, `"changed":true`}} {
		original := objectmodel.CommitReceipt{CommandID: "command", Result: objectmodel.CommitResult{Changed: row.state}}
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		if row.field == "" && strings.Contains(string(raw), `"changed"`) || row.field != "" && !strings.Contains(string(raw), row.field) {
			t.Fatalf("optional Boolean changed: %s", raw)
		}
		var one, two objectmodel.CommitReceipt
		if err := json.Unmarshal(raw, &one); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &two); err != nil {
			t.Fatal(err)
		}
		if one != two || one != original {
			t.Fatalf("decoding changed receipt value identity: %#v %#v", one, two)
		}
	}
	var legacy objectmodel.CommitReceipt
	if err := json.Unmarshal([]byte(`{"result":{"newCommit":"old"}}`), &legacy); err != nil || legacy.Result.Changed != objectmodel.ChangeUnknown {
		t.Fatalf("legacy false inferred: %#v %v", legacy, err)
	}
	for _, raw := range []string{`"UNCHANGED"`, `0`, `[]`, `{}`} {
		var verdict objectmodel.ChangeState
		if err := json.Unmarshal([]byte(raw), &verdict); err == nil {
			t.Fatalf("non-Boolean changed accepted: %s", raw)
		}
	}
}
