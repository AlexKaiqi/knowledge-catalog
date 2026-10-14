package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/client"
	inv "kc/invocation"
	snapshotclient "kc/snapshot/client"
)

func TestTypedTaskResolutionPreservesAnonymousDefinitionAndOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/snapshot/v1/datasets:resolve" || r.Header.Get("X-Kc-As") != "agent:test" || r.Header.Get("X-Kc-Request-Id") != "task-A" {
			t.Errorf("typed task request lost route or identity: %s %s %#v", r.Method, r.URL, r.Header)
		}
		var request snapshotclient.KnowledgeSetRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Dataset != "" || request.Revision != 2 || request.Metadata["custom"] != "value" || len(request.Sources) != 2 {
			t.Errorf("anonymous task changed its selected definition: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2, "pinId": "fixed-task", "repositories": map[string]string{"repo-A": "fixed"}})
	}))
	defer server.Close()
	remote, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "agent:test"}}); err != nil {
		t.Fatal(err)
	}
	result, err := runRemoteTaskResolution(context.Background(), remote, "pin", map[string]FlagValue{
		"revision": " 2 ", "source": "repo-A@reference@docs", "file-source": "repo-A@raw.bin@exports/value.bin", "metadata": `{"custom":"value"}`,
	}, client.RequestOptions{RequestID: "task-A"})
	if err != nil {
		t.Fatal(err)
	}
	pin, ok := result.(inv.TaskKnowledgeSetPin)
	if !ok || pin.PinID != "fixed-task" || pin.Definition == nil || pin.Definition.Revision != 2 || len(pin.Definition.Sources) != 2 || pin.Definition.Metadata["custom"] != "value" {
		t.Fatalf("typed task did not preserve the replay definition: %#v", result)
	}
	if _, exists := cliSurface["pin"]; exists {
		t.Fatal("typed task restoration reopened a retired public CLI command")
	}
}
