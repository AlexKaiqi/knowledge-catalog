package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
)

func httpAny(t *testing.T, srv *httptest.Server, verb string, body any, as string) (int, any, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/"+verb, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if as != "" {
		req.Header.Set("X-Kc-As", as)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("json %s: %s", err, raw)
		}
	}
	return res.StatusCode, payload, raw
}

func httpJSON(t *testing.T, srv *httptest.Server, verb string, body any, as string) (int, map[string]any) {
	t.Helper()
	code, payload, raw := httpAny(t, srv, verb, body, as)
	if payload == nil {
		return code, nil
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("want object, got %s", raw)
	}
	return code, m
}

func TestHTTPFacadeWriteRead(t *testing.T) {
	home := t.TempDir()
	srv := httptest.NewServer(cli.HTTPHandler(home))
	t.Cleanup(srv.Close)

	page, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != 200 || !bytes.Contains(html, []byte("kc 工作台")) {
		t.Fatalf("console page: %d %s", page.StatusCode, html[:min(200, len(html))])
	}
	for _, verb := range []string{"search", "checkout", "inspect", "store-ls", "describe-index", "index-sync"} {
		if !bytes.Contains(html, []byte(verb)) {
			t.Fatalf("console missing verb %s", verb)
		}
	}

	code, init := httpJSON(t, srv, "init", map[string]any{"catalog": "acme/catalog", "home": "/tmp/should-not-win"}, "")
	if code != 200 {
		t.Fatal(init)
	}
	if _, ok := init["home"]; ok {
		t.Fatal("init must not echo local --home", init)
	}
	if init["catalog"] != "kr://acme/catalog" {
		t.Fatal(init)
	}
	code, dumped := httpJSON(t, srv, "read", map[string]any{"catalog": true}, "")
	if code != 200 || dumped["catalogId"] != "kr://acme/catalog" {
		t.Fatal(dumped)
	}
	if _, err := os.Stat(filepath.Join(home, "layout.yaml")); err != nil {
		t.Fatal("process --home unused", err)
	}
	if _, err := os.Stat("/tmp/should-not-win/layout.yaml"); err == nil {
		t.Fatal("JSON home must not initialize a workspace")
	}

	alice := "kr://acme/personals/alice"
	if code, body := httpJSON(t, srv, "repo-add", map[string]any{"repo": alice}, ""); code != 200 {
		t.Fatal(body)
	}
	code, put := httpJSON(t, srv, "put", map[string]any{
		"command-id":  "http-1",
		"repo":        alice,
		"object":      "runbooks/payment-oncall",
		"value":       map[string]any{"text": "check freeze window"},
		"origin-kind": "SOURCE",
	}, "")
	if code != 200 {
		t.Fatal(put)
	}
	result, _ := put["result"].(map[string]any)
	if result["newCommit"] == nil {
		t.Fatal(put)
	}

	code, read := httpJSON(t, srv, "read", map[string]any{
		"repo":   alice,
		"object": "runbooks/payment-oncall",
		"ref":    "refs/heads/main",
	}, "")
	if code != 200 {
		t.Fatal(read)
	}
	value, _ := read["value"].(map[string]any)
	if value["text"] != "check freeze window" {
		t.Fatal(read)
	}

	st, err := srv.Client().Get(srv.URL + "/v1/_state")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(st.Body)
	st.Body.Close()
	if st.StatusCode != 200 {
		t.Fatal(string(raw))
	}
	if !bytes.Contains(raw, []byte(alice)) {
		t.Fatal(string(raw))
	}
	if !bytes.Contains(raw, []byte("runbooks/payment-oncall")) {
		t.Fatalf("tree should list catalog files: %s", raw)
	}

	if code, body := httpJSON(t, srv, "define-workspace", map[string]any{
		"workspace": "payments-agent",
		"revision":  1,
		"source":    alice + "=refs/heads/main",
	}, ""); code != 200 {
		t.Fatal(body)
	}
	code, federated, rawFed := httpAny(t, srv, "read", map[string]any{
		"workspace": "payments-agent",
		"object":    "runbooks/payment-oncall",
	}, "")
	if code != 200 {
		t.Fatal(federated)
	}
	rows, ok := federated.([]any)
	if !ok || len(rows) != 1 {
		t.Fatal(string(rawFed))
	}
}

func TestHTTPFacadeAsForbidden(t *testing.T) {
	home := t.TempDir()
	srv := httptest.NewServer(cli.HTTPHandler(home))
	t.Cleanup(srv.Close)
	httpJSON(t, srv, "init", map[string]any{"catalog": "kr://acme/catalog"}, "")
	alice := "kr://acme/personals/alice"
	httpJSON(t, srv, "repo-add", map[string]any{"repo": alice}, "")
	code, body := httpJSON(t, srv, "put", map[string]any{
		"command-id": "x",
		"repo":       alice,
		"object":     "a",
		"value":      map[string]any{"n": 1},
	}, "bot")
	if code != http.StatusForbidden {
		t.Fatalf("status %d body %#v", code, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "FORBIDDEN" {
		t.Fatal(body)
	}
}

func TestHTTPFacadeRejectsServeVerb(t *testing.T) {
	srv := httptest.NewServer(cli.HTTPHandler(t.TempDir()))
	t.Cleanup(srv.Close)
	code, body := httpJSON(t, srv, "serve", map[string]any{}, "")
	if code != http.StatusNotFound {
		t.Fatal(code, body)
	}
}

func TestHTTPFacadeUnknownVerb(t *testing.T) {
	srv := httptest.NewServer(cli.HTTPHandler(t.TempDir()))
	t.Cleanup(srv.Close)
	code, body := httpJSON(t, srv, "not-a-verb", map[string]any{}, "")
	if code != 400 && code != 404 {
		t.Fatal(code)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "USAGE_INVALID" {
		t.Fatalf("error envelope %#v", body)
	}
	if !strings.Contains(cli.Help, "kc serve") {
		t.Fatal("help should mention serve")
	}
}

func TestStateTreeHasNoCatalogOpsStream(t *testing.T) {
	home := t.TempDir()
	srv := httptest.NewServer(cli.HTTPHandler(home))
	t.Cleanup(srv.Close)
	if code, body := httpJSON(t, srv, "init", map[string]any{"catalog": "kr://acme/catalog"}, ""); code != 200 {
		t.Fatal(body)
	}
	res, err := srv.Client().Get(srv.URL + "/v1/_state")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(string(raw))
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	tree, _ := payload["tree"].(map[string]any)
	roots, _ := tree["roots"].([]any)
	var sawCatalog, sawOpsRepo, sawCatalogOps bool
	for _, item := range roots {
		row, _ := item.(map[string]any)
		kind, _ := row["kind"].(string)
		id, _ := row["id"].(string)
		if kind == "repo" && id == "kr://acme/ops" {
			sawOpsRepo = true
		}
		if kind == "catalog" && id == "kr://acme/catalog" {
			sawCatalog = true
			streams, _ := row["streams"].([]any)
			if len(streams) > 0 {
				sawCatalogOps = true
			}
		}
	}
	if !sawCatalog {
		t.Fatalf("catalog root missing: %s", raw)
	}
	if sawCatalogOps {
		t.Fatal("Catalog must not expose an ops stream")
	}
	if sawOpsRepo {
		t.Fatal("ops must not appear as a repository in the tree")
	}
	if _, ok := tree["sidecars"]; ok {
		t.Fatal("local sidecar files must not appear in the Catalog tree")
	}
}

func TestHTTPFacadeStampsCatalogGit(t *testing.T) {
	home := t.TempDir()
	srv := httptest.NewServer(cli.HTTPHandler(home))
	t.Cleanup(srv.Close)
	if code, body := httpJSON(t, srv, "init", map[string]any{"catalog": "kr://acme/catalog"}, ""); code != 200 {
		t.Fatal(body)
	}
	core := "kr://acme/public/core"
	if code, body := httpJSON(t, srv, "repo-add", map[string]any{"repo": core}, ""); code != 200 {
		t.Fatal(body)
	}
	code, rule := httpJSON(t, srv, "allow", map[string]any{
		"principal": "agent:payments",
		"cmd":       "define-workspace",
		"catalog":   "kr://acme/catalog",
	}, "")
	if code != 200 {
		t.Fatal(rule)
	}
	rawBody := []byte(`{"catalog":"kr://acme/catalog","workspace":"duty","revision":1,"source":"kr://acme/public/core=refs/heads/main"}`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/define-workspace", bytes.NewReader(rawBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kc-As", "agent:payments")
	req.Header.Set("X-Kc-Request-Id", "gw-99")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(string(out))
	}
	code, trail := httpJSON(t, srv, "audit", map[string]any{}, "")
	if code != 200 {
		t.Fatal(trail)
	}
	entries, _ := trail["entries"].([]any)
	var saw bool
	for _, item := range entries {
		row, _ := item.(map[string]any)
		msg, _ := row["message"].(string)
		if !strings.HasPrefix(msg, "define-workspace") {
			continue
		}
		saw = true
		if row["author"] != "agent:payments" || row["requestId"] != "gw-99" || row["ruleId"] != rule["id"] {
			t.Fatalf("%#v rule %#v", row, rule)
		}
	}
	if !saw {
		t.Fatal(entries)
	}
}
