package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

func TestModuleGrantListKeepsTargetRulesAndDropsLedgerHeaders(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodGet || request.URL.Path != "/admin/v1/grants" || request.URL.Query().Get("repository") != "kr://x" || request.Header.Get("X-Kc-Request-Id") != "grant-inventory" {
			t.Errorf("grant inventory changed its request: %s %s", request.Method, request.URL.RequestURI())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":       2,
			"initialGrants": map[string]any{"kr://x": "digest"},
			"rules": []any{
				map[string]any{"id": "alw_1", "principal": "bot", "repo": "kr://x", "actions": []any{"object.read"}},
				map[string]any{"id": "alw_2", "principal": "bot", "actions": []any{"inventory.read"}},
			},
		})
	}))
	defer server.Close()
	sdk, err := client.New(client.Config{BaseURL: server.URL, Transport: &testTransport{client: server.Client()}})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := snapshotcli.New(sdk, snapshotcli.Context{}).Execute(context.Background(), "grant list", map[string]any{"repo": "kr://x"}, client.RequestOptions{RequestID: "grant-inventory"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["version"]; ok {
		t.Fatalf("grant list kept allow file header: %#v", listed)
	}
	if _, ok := got["initialGrants"]; ok || len(got) != 1 {
		t.Fatalf("grant list exposed ledger state beyond rules: %#v", listed)
	}
	rules, _ := got["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("grant list --repo filter: %#v", listed)
	}
	rule, _ := rules[0].(map[string]any)
	if rule["id"] != "alw_1" {
		t.Fatalf("grant list kept the wrong rule: %#v", listed)
	}
	if calls != 1 {
		t.Fatalf("grant inventory was retried or queried twice: %d", calls)
	}
}
