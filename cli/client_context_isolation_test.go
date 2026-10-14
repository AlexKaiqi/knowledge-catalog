package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Retired host files must neither override formal client coordinates nor make
// a business command callable without an explicitly configured Server.
func TestClientCoordinatesIgnoreRetiredHostContext(t *testing.T) {
	for _, group := range []string{"tasks", "projects"} {
		for _, mode := range []string{"argv", "environment", "no-server"} {
			t.Run(group+"/"+mode, func(t *testing.T) {
				isolateLoginConfig(t)
				t.Setenv("KC_DATASET", "")
				t.Setenv("KC_CATALOG", "")
				var retiredCalls atomic.Int32
				retired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					retiredCalls.Add(1)
					_ = json.NewEncoder(w).Encode([]any{})
				}))
				t.Cleanup(retired.Close)
				seen := make(chan map[string]any, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/object/v1/objects:read" || r.Header.Get("X-Kc-As") != "agent:consumer" {
						t.Errorf("client coordinates changed: %s as=%q", r.URL.Path, r.Header.Get("X-Kc-As"))
						http.Error(w, "unexpected client coordinates", http.StatusBadRequest)
						return
					}
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					select {
					case seen <- request:
					default:
						t.Error("client sent a duplicate read request")
						http.Error(w, "duplicate read", http.StatusBadRequest)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"reference": request["reference"], "repository": "kr://acme/source", "address": map[string]any{"objectId": "note/current"}})
				}))
				t.Cleanup(server.Close)
				root, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				home := t.TempDir()
				t.Setenv("KC_HOME", home)
				contextDir := filepath.Join(home, group, "retired")
				if err := os.MkdirAll(contextDir, 0700); err != nil {
					t.Fatal(err)
				}
				context, err := json.Marshal(map[string]any{
					"version": 1, "root": root, "readOnly": true,
					"server": retired.URL, "principal": "agent:retired", "dataset": "retired",
					"pin": map[string]any{"setId": "retired", "repositories": []any{}},
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(contextDir, "context.json"), context, 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fcurrent", "--dataset", "current"}
				switch mode {
				case "argv":
					args = append(args, "--server", server.URL, "--as", "agent:consumer")
				case "environment":
					t.Setenv("KC_SERVER_URL", server.URL)
					t.Setenv("KC_AS", "agent:consumer")
				case "no-server":
					args = []string{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fcurrent"}
				}
				result := Run(args)
				if mode == "no-server" {
					if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
						t.Fatalf("retired context supplied a Server: %#v", result)
					}
				} else {
					if result.Status != 0 {
						t.Fatal(result.Stdout)
					}
					var request map[string]any
					select {
					case request = <-seen:
					default:
						t.Fatal("client did not call the configured typed API")
					}
					if request["dataset"] != "current" || request["reference"] != "urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fcurrent" {
						t.Fatalf("client object coordinates changed: %#v", request)
					}
					for _, key := range []string{"catalog", "pin", "definition"} {
						if _, ok := request[key]; ok {
							t.Fatalf("retired %s leaked into request: %#v", key, request)
						}
					}
				}
				if retiredCalls.Load() != 0 {
					t.Fatalf("retired endpoint received %d requests", retiredCalls.Load())
				}
			})
		}
	}
}
