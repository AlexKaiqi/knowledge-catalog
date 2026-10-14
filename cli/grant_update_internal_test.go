package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/client"
	"kc/invoke"
	"kc/kernel"
)

const observedGrantJSON = `{"id":"rule-A","principal":"reader","actions":["file.read"],"repo":"repo-A","dataset":"published","ref":"main","object":"opaque/*","aspect":"canonical","sharedBy":"publisher"}`

func grantUpdateFiles(t *testing.T, replacement, observed string) map[string]FlagValue {
	t.Helper()
	dir := t.TempDir()
	flags := map[string]FlagValue{"id": "rule-A", "file": filepath.Join(dir, "replacement.json"), "expected-file": filepath.Join(dir, "observed.json")}
	for name, body := range map[string]string{"file": replacement, "expected-file": observed} {
		if err := os.WriteFile(invoke.FlagString(flags, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return flags
}

func grantUpdateTestClient(t *testing.T, handler http.Handler) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	remote, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "manager"}}); err != nil {
		t.Fatal(err)
	}
	return remote
}

func TestGrantUpdateTransmitsObservedRuleWithoutRefreshing(t *testing.T) {
	calls := 0
	remote := grantUpdateTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPut || r.URL.Path != "/admin/v1/grants/rule-A" || r.Header.Get("X-Kc-As") != "manager" || r.Header.Get("X-Kc-Request-Id") != "request-A" {
			t.Errorf("update changed its operation or caller: %s %s %#v", r.Method, r.URL.Path, r.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		want := map[string]any{
			"principal": "other-reader", "actions": []any{"file.read", "dataset.resolve"}, "dataset": "next",
			"expected": map[string]any{"id": "rule-A", "principal": "reader", "actions": []any{"file.read"}, "repository": "repo-A", "dataset": "published", "ref": "main", "object": "opaque/*", "aspect": "canonical", "sharedBy": "publisher"},
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("update lost the observed scope, retained removed qualifiers or leaked provenance into replacement: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"rule-A","principal":"other-reader","actions":["file.read","dataset.resolve"],"dataset":"next","sharedBy":"publisher"}`))
	}))
	flags := grantUpdateFiles(t, `{"id":"rule-A","principal":"other-reader","actions":["file.read","dataset.resolve"],"dataset":"next","sharedBy":"publisher"}`, observedGrantJSON)
	result, err := executeSnapshotCommand(context.Background(), remote, "grant update", flags, client.RequestOptions{RequestID: "request-A"})
	if err != nil || calls != 1 {
		t.Fatalf("update must send one PUT without obtaining a new expected rule: calls=%d result=%#v err=%v", calls, result, err)
	}
	if result.(map[string]any)["id"] != "rule-A" {
		t.Fatalf("update discarded the unchanged rule identity: %#v", result)
	}
}

func TestGrantUpdateRejectsInvalidRuleFilesBeforeTransport(t *testing.T) {
	valid := `{"principal":"reader","actions":["file.read"]}`
	for _, test := range []struct {
		name        string
		replacement string
		observed    string
	}{
		{"unknown", `{"principal":"reader","actions":["file.read"],"typo":"A"}`, observedGrantJSON},
		{"HTTP-vocabulary", `{"principal":"reader","actions":["file.read"],"repository":"A"}`, observedGrantJSON},
		{"case-alias", `{"principal":"reader","actions":["file.read"],"SharedBy":"other"}`, observedGrantJSON},
		{"legacy-command", `{"principal":"reader","actions":["file.read"],"cmds":["read"]}`, observedGrantJSON},
		{"duplicate-field", `{"principal":"reader","actions":["file.read"],"id":"other","id":"rule-A"}`, observedGrantJSON},
		{"null-coordinate", `{"principal":"reader","actions":["file.read"],"repo":null}`, observedGrantJSON},
		{"inventory-wrapper", `{"rules":[]}`, observedGrantJSON},
		{"array", `[]`, observedGrantJSON},
		{"null", `null`, observedGrantJSON},
		{"trailing-value", valid + `{}`, observedGrantJSON},
		{"principal-missing", `{"actions":["file.read"]}`, observedGrantJSON},
		{"actions-missing", `{"principal":"reader"}`, observedGrantJSON},
		{"changed-ID", `{"id":"other","principal":"reader","actions":["file.read"]}`, observedGrantJSON},
		{"cleared-ID", `{"id":"","principal":"reader","actions":["file.read"]}`, observedGrantJSON},
		{"changed-source", `{"principal":"reader","actions":["file.read"],"sharedBy":"other"}`, observedGrantJSON},
		{"cleared-source", `{"principal":"reader","actions":["file.read"],"sharedBy":""}`, observedGrantJSON},
		{"changed-share", `{"principal":"reader","actions":["file.read"],"shareId":"share"}`, observedGrantJSON},
		{"unknown-observed", valid, `{"id":"rule-A","principal":"reader","actions":["file.read"],"canManage":true}`},
		{"wrong-observed-ID", valid, `{"id":"other","principal":"reader","actions":["file.read"]}`},
		{"observed-wrapper", valid, `{"rules":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			remote := grantUpdateTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusTeapot) }))
			_, err := executeSnapshotCommand(context.Background(), remote, "grant update", grantUpdateFiles(t, test.replacement, test.observed), client.RequestOptions{})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid || calls != 0 {
				t.Fatalf("invalid or forged rule reached transport: calls=%d err=%v", calls, err)
			}
		})
	}
}
