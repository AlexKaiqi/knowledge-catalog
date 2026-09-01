package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

func TestTypedRefineQueryDoesNotUseCurrentRequestTraceAsFilter(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	principal := "agent:audit"
	if err := cli.WriteAllow(home, cli.AllowFile{Version: 2, Rules: []cli.AllowRule{
		{ID: "audit", Principal: principal, Actions: []string{"audit.read"}},
		{ID: "feedback", Principal: principal, Actions: []string{"feedback.write"}, Catalog: "kr://acme/catalog", Workspace: "agent"},
		{ID: "workspace", Principal: principal, Actions: []string{"workspace.consume"}, Catalog: "kr://acme/catalog", Workspace: "agent"},
	}}); err != nil {
		t.Fatal(err)
	}
	store := observability.NewFileStore(home)
	identity := observability.IdentityContext{Principal: "agent:answer"}
	trace := observability.TraceContext{TraceID: "trace-original", SpanID: "span-original"}
	accessID, err := store.RecordAccessReceipt(observability.AccessEvent{
		Identity: identity, Trace: trace, Action: "knowledge.rerank", Workspace: "agent",
		Decision: "ALLOW", Result: "RESOLVED", Knowledge: []observability.KnowledgeAccess{},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := knowledge.KnowledgeRef{Repository: "kr://acme/runbooks", Object: "runbook/refund"}
	value := map[string]any{"body": "refund timeout"}
	refineID, err := store.RecordRefineReceipt(observability.RefineEvent{
		AccessEvidenceID: accessID, Identity: identity, Trace: trace, Action: "knowledge.rerank", Workspace: "agent",
		SearchView:      observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{ref.Repository: "c1"}},
		Spec:            observability.RefineSpec{SpecRef: "urn:rank:test", Revision: 1, Operator: "SEMANTIC_RERANK", Criterion: "refund relevance"},
		CandidateDigest: "window-1", ProjectedBytes: 32,
		Candidates: []observability.RefineCandidate{{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}, Value: value,
			ValueDigest: kernel.CanonicalDigest(value), OriginalRank: 1,
		}},
		Outcome: "COMPLETED", ModelOutput: &observability.RefineModelOutput{
			Provider: "llm-native", Model: "judge", Groups: []observability.RefineRankGroup{{Rank: 1, Refs: []knowledge.KnowledgeRef{ref}}}, Unjudged: []knowledge.KnowledgeRef{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	status, payload := semanticHTTPAs(t, server, "/operations/v1/refine-log:query", principal, map[string]any{"evidenceId": refineID})
	if status != http.StatusOK || len(payload["entries"].([]any)) != 1 {
		t.Fatalf("refine query status=%d payload=%#v", status, payload)
	}
	status, payload = semanticHTTPAs(t, server, "/operations/v1/feedback", principal, map[string]any{
		"workspace": "agent", "traceId": trace.TraceID, "outcome": "answered", "refineEvidenceId": refineID,
		"selectedRefs": []any{map[string]any{"repository": ref.Repository, "object": "runbook/outside-window"}},
	})
	if status != http.StatusBadRequest || asMap(t, payload["error"])["code"] != "USAGE_INVALID" {
		t.Fatalf("out-of-window feedback status=%d payload=%#v", status, payload)
	}
	status, payload = semanticHTTPAs(t, server, "/operations/v1/feedback", principal, map[string]any{
		"workspace": "agent", "traceId": trace.TraceID, "outcome": "answered", "refineEvidenceId": refineID,
		"answer": "inspect the timeout", "selectedRefs": []any{map[string]any{"repository": ref.Repository, "object": ref.Object}},
	})
	if status != http.StatusOK || payload["recorded"] != true {
		t.Fatalf("feedback status=%d payload=%#v", status, payload)
	}
	traceView, err := store.Trace(trace.TraceID)
	if err != nil || len(traceView.Entries) != 3 {
		t.Fatalf("trace after feedback = %#v err=%v", traceView, err)
	}
	feedback := traceView.Entries[2].Feedback
	if feedback == nil || feedback.Trace.TraceID != trace.TraceID || feedback.SubmissionTrace == nil || feedback.SubmissionTrace.TraceID == trace.TraceID {
		t.Fatalf("feedback target/submission traces = %#v", feedback)
	}
	submissionView, err := store.Trace(feedback.SubmissionTrace.TraceID)
	if err != nil || len(submissionView.Entries) != 1 || submissionView.Entries[0].Kind != "feedback" {
		t.Fatalf("feedback submission trace = %#v err=%v", submissionView, err)
	}
}

func TestTypedRetrievalEvidenceQueryAndTraining(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	agent, user := "agent:retrieval-audit", "user:retrieval-reviewer"
	rules := []cli.AllowRule{}
	for _, principal := range []string{agent, user} {
		rules = append(rules,
			cli.AllowRule{ID: principal + "-audit", Principal: principal, Actions: []string{"audit.read"}},
			cli.AllowRule{ID: principal + "-feedback", Principal: principal, Actions: []string{"feedback.write"}, Catalog: "kr://acme/catalog", Workspace: "agent"},
			cli.AllowRule{ID: principal + "-workspace", Principal: principal, Actions: []string{"workspace.consume"}, Catalog: "kr://acme/catalog", Workspace: "agent"},
		)
	}
	if err := cli.WriteAllow(home, cli.AllowFile{Version: 2, Rules: rules}); err != nil {
		t.Fatal(err)
	}
	store := observability.NewFileStore(home)
	identity := observability.IdentityContext{Principal: agent}
	trace := observability.TraceContext{TraceID: "trace-retrieval-training", SpanID: "span-search"}
	ref := knowledge.KnowledgeRef{Repository: "kr://acme/runbooks", Object: "runbook/refund"}
	accessID, err := store.RecordAccessReceipt(observability.AccessEvent{
		Identity: identity, Trace: trace, Action: "knowledge.search", Workspace: "agent", Decision: "ALLOW", Result: "RESOLVED",
	})
	if err != nil {
		t.Fatal(err)
	}
	logical := map[string]any{"query": "refund timeout", "limit": 10}
	candidates := []observability.RetrievalCandidate{{
		KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}, Rank: 1,
		ValueDigest: "candidate-value",
		Evidence:    []observability.RetrievalLaneEvidence{{Provider: "opensearch", Lane: "lexical", Guarantee: "exact", LocalRank: 1}},
	}}
	retrievalID, err := store.RecordRetrievalReceipt(observability.RetrievalEvent{
		AccessEvidenceID: accessID, Identity: identity, Trace: trace, Action: "knowledge.search", Workspace: "agent",
		Operator: observability.RetrievalOperatorSearch, LogicalRequest: logical, RequestDigest: kernel.CanonicalDigest(logical),
		SearchView:   observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{ref.Repository: "c1"}},
		Completeness: "complete", Execution: observability.RetrievalExecution{Candidates: 1, Hydrated: 1},
		Candidates: candidates, CandidateDigest: kernel.CanonicalDigest(candidates), Outcome: "COMPLETED",
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := map[string]any{"body": "refund timeout"}
	refineID, err := store.RecordRefineReceipt(observability.RefineEvent{
		AccessEvidenceID: accessID, RetrievalEvidenceID: retrievalID, Identity: identity, Trace: trace,
		Action: "knowledge.rerank", Workspace: "agent",
		SearchView:      observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{ref.Repository: "c1"}},
		Spec:            observability.RefineSpec{SpecRef: "urn:rank:retrieval", Revision: 1, Operator: "SEMANTIC_RERANK", Criterion: "refund relevance"},
		CandidateDigest: "refine-window", ProjectedBytes: 64,
		Candidates: []observability.RefineCandidate{{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: "c1"}, Value: projected,
			ValueDigest: kernel.CanonicalDigest(projected), OriginalRank: 1,
		}},
		Outcome: "COMPLETED", ModelOutput: &observability.RefineModelOutput{
			Provider: "llm-native", Model: "judge", Groups: []observability.RefineRankGroup{{Rank: 1, Refs: []knowledge.KnowledgeRef{ref}}}, Unjudged: []knowledge.KnowledgeRef{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	status, payload := semanticHTTPAs(t, server, "/operations/v1/retrieval-log:query", agent, map[string]any{
		"evidenceId": retrievalID, "operator": "SEARCH", "provider": "opensearch", "outcome": "COMPLETED",
	})
	if status != http.StatusOK || len(payload["entries"].([]any)) != 1 {
		t.Fatalf("retrieval query status=%d payload=%#v", status, payload)
	}
	selected := []any{map[string]any{"repository": ref.Repository, "object": ref.Object}}
	status, payload = semanticHTTPAs(t, server, "/operations/v1/feedback", agent, map[string]any{
		"workspace": "agent", "traceId": trace.TraceID, "outcome": "answered", "refineEvidenceId": refineID,
		"answer": "Inspect the refund timeout.", "selectedRefs": selected,
	})
	if status != http.StatusOK || payload["recorded"] != true || payload["retrievalEvidenceId"] != retrievalID {
		t.Fatalf("retrieval answer status=%d payload=%#v", status, payload)
	}
	status, payload = semanticHTTPAs(t, server, "/operations/v1/feedback", user, map[string]any{
		"workspace": "agent", "traceId": trace.TraceID, "outcome": "accepted", "refineEvidenceId": refineID,
	})
	if status != http.StatusOK || payload["recorded"] != true {
		t.Fatalf("retrieval acceptance status=%d payload=%#v", status, payload)
	}
	status, payload = semanticHTTPAs(t, server, "/operations/v1/retrieval-training:query", agent, map[string]any{"evidenceId": retrievalID})
	samples := payload["samples"].([]any)
	if status != http.StatusOK || len(samples) != 1 || asMap(t, samples[0])["trainingEligible"] != true {
		t.Fatalf("retrieval training status=%d payload=%#v", status, payload)
	}
}

func TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing(t *testing.T) {
	handler := cli.HTTPHandlerWithOptions(testkit.TempDir(t), cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	formal := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/identity/v1/whoami"},
		{http.MethodGet, "/catalog/v1/catalogs"},
		{http.MethodPost, "/knowledge/v1/objects:read"},
		{http.MethodPost, "/knowledge/v1/schemas:page"},
		{http.MethodPost, "/knowledge/v1/search:rerank"},
		{http.MethodPost, "/knowledge/v1/rerank"},
		{http.MethodPost, "/knowledge/v1/resources:access"},
		{http.MethodPost, "/workspace-files/v1/tree:list"},
		{http.MethodPost, "/writer/v1/repositories/repo/commits"},
		{http.MethodPost, "/governance/v1/proposals"},
		{http.MethodGet, "/admin/v1/grants"},
		{http.MethodPost, "/operations/v1/projections:sync"},
	}
	for _, endpoint := range formal {
		request, err := http.NewRequest(endpoint.method, server.URL+endpoint.path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
			t.Fatalf("formal route is not registered: %s %s (%d)", endpoint.method, endpoint.path, response.StatusCode)
		}
	}

	retired := []string{
		"/v1/read", "/v1/init", "/knowledge/v1/list", "/vfs-read",
		"/workspace-files/v1/files:write", "/local/v1/init",
	}
	for _, path := range retired {
		request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("retired route %s returned %d, want 404", path, response.StatusCode)
		}
	}
}

func TestAppendAndStreamSurfacesStayAbsent(t *testing.T) {
	home := testkit.TempDir(t)
	for _, command := range []string{"append", "stream"} {
		result := kc(home, command)
		if result.Status == 0 || !bytes.Contains([]byte(result.Stdout), []byte("unknown command")) {
			t.Fatalf("kc %s unexpectedly exists: %#v", command, result)
		}
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for _, path := range []string{
		"/writer/v1/repositories/repo/append",
		"/knowledge/v1/stream",
		"/operations/v1/streams:append",
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("retired route %s returned %d, want 404", path, response.StatusCode)
		}
	}
}

func TestTypedServiceRequestsRejectUnknownFieldsAndTrailingJSON(t *testing.T) {
	handler := cli.HTTPHandlerWithOptions(testkit.TempDir(t), cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for name, body := range map[string]string{
		"unknown":  `{"workspace":"agent","object":"x","flags":"must-not-exist"}`,
		"trailing": `{"workspace":"agent","object":"x"} {}`,
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/objects:read", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", "agent:surface-test")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		var envelope map[string]any
		if response.StatusCode != http.StatusBadRequest || json.Unmarshal(raw, &envelope) != nil || asMap(t, envelope["error"])["code"] != "USAGE_INVALID" {
			t.Fatalf("%s status=%d body=%s", name, response.StatusCode, raw)
		}
	}
}

func TestXKcAsUsesTheSameAuthorizationRulesAsCLI(t *testing.T) {
	home := testkit.TempDir(t)
	repository := "kr://acme/public/http-auth"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "repo-add", "--repo", repository))
	body(t, kc(home, "put", "--command-id", "seed", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"one"}`))
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main"))
	body(t, kc(home, "allow", "--principal", "agent:http", "--cmd", "read-workspace", "--catalog", "kr://acme/catalog", "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:http", "--cmd", "read", "--repo", repository))

	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for principal, want := range map[string]int{"agent:http": http.StatusOK, "agent:intruder": http.StatusForbidden} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/objects:read",
			bytes.NewBufferString(`{"workspace":"agent","object":"Policy:one"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Kc-As", principal)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("principal=%s status=%d want=%d", principal, response.StatusCode, want)
		}
	}
}
