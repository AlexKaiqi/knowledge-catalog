package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestAgentDelegatedAccessTraceFeedbackAndHitmap(t *testing.T) {
	home := testkit.TempDir(t)
	repoID := "kr://acme/public/semantics"
	catalogID := "kr://acme/catalog"
	workspaceID := "finance-board"
	agent := "agent:finance-analyst-v3"
	user := "user:kaiqidong"

	body(t, kc(home, "init", "--catalog", catalogID))
	body(t, kc(home, "repo-add", "--repo", repoID))
	body(t, kc(home, "put", "--command-id", "schema", "--repo", repoID,
		"--object", "schema/metric.definition",
		"--value", `{"entity":"Metric","pattern":"record","fields":{"description":{"type":"string","access":["text"]}}}`))
	body(t, kc(home, "put", "--command-id", "metric", "--repo", repoID,
		"--object", "Metric:gmv", "--value", `{"description":"governed gross merchandise value"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspaceID, "--revision", "1",
		"--source", repoID+"=refs/heads/main"))
	body(t, kc(home, "allow", "--principal", agent, "--cmd", "read-workspace",
		"--catalog", catalogID, "--workspace", workspaceID))
	body(t, kc(home, "allow", "--principal", agent, "--cmd", "read", "--repo", repoID))

	identity := []string{"--as", agent, "--on-behalf-of", user, "--request-id", "req-42",
		"--trace-id", "trace-42", "--session-id", "session-7"}
	readArgs := append([]string{"read", "--workspace", workspaceID, "--object", "Metric:gmv", "--span-id", "span-read"}, identity...)
	readValues := body(t, kc(home, readArgs...)).([]any)
	if len(readValues) != 1 {
		t.Fatal(readValues)
	}
	returnedCommit, _ := asMap(t, readValues[0])["commit"].(string)
	if returnedCommit == "" {
		t.Fatal("read response must identify the knowledge version", readValues[0])
	}
	searchArgs := append([]string{"search", "--workspace", workspaceID, "--query", "merchandise", "--span-id", "span-search", "--parent-span-id", "span-read"}, identity...)
	searchResult := asMap(t, body(t, kc(home, searchArgs...)))
	searchValues := searchResult["hits"].([]any)
	if len(searchValues) != 1 {
		t.Fatal(searchValues)
	}

	who := asMap(t, body(t, kc(home, "whoami", "--as", agent, "--on-behalf-of", user)))
	if who["principal"] != agent || who["onBehalfOf"] != user {
		t.Fatal(who)
	}
	body(t, kc(home, "record-feedback", "--workspace", workspaceID,
		"--trace-id", "trace-42", "--session-id", "session-7", "--as", agent,
		"--on-behalf-of", user, "--outcome", "helpful", "--message", "answer accepted"))

	log := asMap(t, body(t, kc(home, "access-log", "--trace-id", "trace-42")))
	entries := log["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("want read + search access, got %#v", entries)
	}
	for _, raw := range entries {
		event := asMap(t, raw)
		id := asMap(t, event["identity"])
		traceContext := asMap(t, event["trace"])
		if id["principal"] != agent || id["onBehalfOf"] != user || event["occurredAt"] == "" || event["pinId"] == "" ||
			event["decision"] != "ALLOW" || event["result"] != "RESOLVED" || traceContext["traceId"] != "trace-42" || traceContext["spanId"] == "" {
			t.Fatalf("identity/time/pin missing: %#v", event)
		}
		knowledge := event["knowledge"].([]any)
		if len(knowledge) != 1 {
			t.Fatalf("versioned target missing: %#v", event)
		}
		ref := asMap(t, asMap(t, knowledge[0])["knowledgeRef"])
		if ref["repository"] != repoID || ref["object"] != "Metric:gmv" || ref["commit"] != returnedCommit {
			t.Fatalf("wrong pinned knowledge ref: %#v", ref)
		}
	}

	trace := asMap(t, body(t, kc(home, "trace", "--trace-id", "trace-42")))
	traceEntries := trace["entries"].([]any)
	if len(traceEntries) != 3 || asMap(t, traceEntries[2])["kind"] != "feedback" {
		t.Fatalf("trace must contain read, search and feedback: %#v", traceEntries)
	}
	hitmap := asMap(t, body(t, kc(home, "hitmap", "--object", "Metric:gmv")))
	hits := hitmap["hits"].([]any)
	if len(hits) != 1 {
		t.Fatal(hits)
	}
	hit := asMap(t, hits[0])
	if hit["hits"] != float64(2) || asMap(t, hit["principals"])[agent] != float64(2) || asMap(t, hit["onBehalfOf"])[user] != float64(2) {
		t.Fatalf("hitmap %#v", hit)
	}

	blocked := "agent:blocked"
	expectCode(t, kc(home, "read", "--workspace", workspaceID, "--object", "Metric:gmv",
		"--as", blocked, "--on-behalf-of", user, "--trace-id", "trace-denied"), "FORBIDDEN")
	denied := asMap(t, body(t, kc(home, "access-log", "--filter-principal", blocked)))
	deniedEntries := denied["entries"].([]any)
	if len(deniedEntries) != 1 || asMap(t, deniedEntries[0])["decision"] != "DENY" {
		t.Fatalf("denied access must be durable: %#v", deniedEntries)
	}
	expectCode(t, kc(home, "record-feedback", "--workspace", workspaceID,
		"--trace-id", "trace-missing", "--as", agent, "--outcome", "helpful"), "PRECONDITION_FAILED")
}

func TestKnowledgeReadFailsClosedWhenAccessEvidenceCannotPersist(t *testing.T) {
	home := testkit.TempDir(t)
	repoID := "kr://acme/public/audited"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "repo-add", "--repo", repoID))
	body(t, kc(home, "put", "--command-id", "seed", "--repo", repoID,
		"--object", "Policy:audit", "--value", `{"body":"must be observed"}`))
	if err := os.Mkdir(filepath.Join(home, "access.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := kc(home, "read", "--repo", repoID, "--ref", "refs/heads/main", "--object", "Policy:audit")
	if result.Status == 0 {
		t.Fatal("a successful facade response must not escape without durable access evidence")
	}
}

func TestHTTPPassesAbstractDelegationAndTraceContext(t *testing.T) {
	home := testkit.TempDir(t)
	repoID := "kr://acme/public/http-observe"
	workspaceID := "http-observe"
	agent, user := "agent:http", "user:delegator"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "repo-add", "--repo", repoID))
	body(t, kc(home, "put", "--command-id", "seed", "--repo", repoID,
		"--object", "Policy:http", "--value", `{"body":"observable"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspaceID, "--revision", "1",
		"--source", repoID+"=refs/heads/main"))
	body(t, kc(home, "allow", "--principal", agent, "--cmd", "read-workspace",
		"--catalog", "kr://acme/catalog", "--workspace", workspaceID))
	body(t, kc(home, "allow", "--principal", agent, "--cmd", "read", "--repo", repoID))

	server := httptest.NewServer(cli.HTTPHandler(home))
	t.Cleanup(server.Close)
	payload, _ := json.Marshal(map[string]any{"workspace": workspaceID, "object": "Policy:http"})
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/read", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kc-As", agent)
	req.Header.Set("X-Kc-On-Behalf-Of", user)
	req.Header.Set("X-Kc-Request-Id", "req-http")
	req.Header.Set("X-Kc-Trace-Id", "trace-http")
	req.Header.Set("X-Kc-Span-Id", "span-http")
	req.Header.Set("X-Kc-Session-Id", "session-http")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read %d: %s", response.StatusCode, raw)
	}

	log := asMap(t, body(t, kc(home, "access-log", "--trace-id", "trace-http")))
	entries := log["entries"].([]any)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
	event := asMap(t, entries[0])
	identity := asMap(t, event["identity"])
	trace := asMap(t, event["trace"])
	if identity["principal"] != agent || identity["onBehalfOf"] != user || event["requestId"] != "req-http" ||
		trace["traceId"] != "trace-http" || trace["spanId"] != "span-http" || trace["sessionId"] != "session-http" {
		t.Fatalf("headers were not preserved: %#v", event)
	}
}
