package access

import (
	"encoding/json"
	"testing"
)

func TestRetiredSearchGrantDoesNotRejectOrWidenHistoricalRules(t *testing.T) {
	if err := ValidateActions([]string{"object.search"}); err == nil {
		t.Fatal("new Repository SEARCH grant accepted after the operation was retired")
	}
	var historic Rule
	if err := json.Unmarshal([]byte(`{"id":"old-search","principal":"reader","repo":"kr://old/source","actions":["object.search"]}`), &historic); err != nil {
		t.Fatalf("retirement rewrote the historical authorization contract: %v", err)
	}
	if len(historic.Actions) != 1 || historic.Actions[0] != "object.search" {
		t.Fatalf("historic receipt actions changed: %#v", historic)
	}
	if _, ok := Match([]Rule{historic}, Query{Principal: "reader", Action: "file.read", Dataset: "published"}); ok {
		t.Fatal("historical SEARCH grant widened into Dataset consumption")
	}
}
