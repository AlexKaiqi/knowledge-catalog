package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// TestLiveServiceProviderConsumerJourney is the deterministic service-MVP
// gate: real Gitea authentication, native Dolt authority, real OpenSearch, two
// independent principals, and HTTP-only role journeys after operator setup.
func TestLiveServiceProviderConsumerJourney(t *testing.T) {
	if testing.Short() {
		t.Skip("live provider/consumer journey belongs to make test-service-e2e")
	}
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		t.Skip("KC_TEST_OPENSEARCH_URL is required")
	}
	giteaURL, adapterToken, run := testkit.GiteaEndpoint(t)
	t.Setenv("KC_GITEA_TOKEN", adapterToken)

	providerLogin, providerPassword := "provider"+run, "Provider-"+run+"!"
	consumerLogin, consumerPassword := "consumer"+run, "Consumer-"+run+"!"
	providerID := createLiveGiteaUser(t, giteaURL, adapterToken, providerLogin, providerPassword)
	consumerID := createLiveGiteaUser(t, giteaURL, adapterToken, consumerLogin, consumerPassword)
	providerAuth := basicAuthorization(providerLogin, providerPassword)
	consumerAuth := basicAuthorization(consumerLogin, consumerPassword)

	home := testkit.TempDir(t)
	catalogID := "kr://service/catalog"
	repositoryID := "kr://service/public/runbooks"
	workspaceID := "agent"
	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "store", "set", "--repository", "dolt", "--index", "opensearch"))
	body(t, kc(home, "local", "store", "set", "--driver", "opensearch", "--url", opensearchURL))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID, "--driver", "dolt"))
	body(t, kc(home, "workspace", "define", "--workspace", workspaceID, "--revision", "1", "--source", repositoryID+"=refs/heads/main"))
	body(t, kc(home, "admin", "grant", "add", "--principal", fmt.Sprintf("gitea:%d", providerID), "--action", "writer.commit,projection.manage", "--repo", repositoryID))
	body(t, kc(home, "admin", "grant", "add", "--principal", fmt.Sprintf("gitea:%d", consumerID), "--action", "workspace.consume,workspace.resolve", "--catalog", catalogID, "--workspace", workspaceID))
	body(t, kc(home, "admin", "grant", "add", "--principal", fmt.Sprintf("gitea:%d", consumerID), "--action", "knowledge.read,knowledge.search", "--repo", repositoryID))

	authenticator, err := cli.NewGiteaAuthenticator(giteaURL, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Authenticator: authenticator})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	status, ready := liveServiceRequest(t, server, http.MethodGet, "/readyz", nil, "")
	if status != http.StatusOK || asMap(t, ready)["status"] != "ready" {
		t.Fatalf("service not ready: status=%d body=%#v", status, ready)
	}
	status, who := liveServiceRequest(t, server, http.MethodGet, "/identity/v1/whoami", nil, providerAuth)
	if status != http.StatusOK || asMap(t, who)["principal"] != fmt.Sprintf("gitea:%d", providerID) {
		t.Fatalf("provider identity: status=%d body=%#v", status, who)
	}
	forged, err := http.NewRequest(http.MethodGet, server.URL+"/identity/v1/whoami", nil)
	if err != nil {
		t.Fatal(err)
	}
	forged.Header.Set("Authorization", providerAuth)
	forged.Header.Set("X-Kc-As", fmt.Sprintf("gitea:%d", consumerID))
	forgedResponse, err := server.Client().Do(forged)
	if err != nil {
		t.Fatal(err)
	}
	_ = forgedResponse.Body.Close()
	if forgedResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("authenticated request accepted forged X-Kc-As: %d", forgedResponse.StatusCode)
	}
	status, _ = liveServiceRequest(t, server, http.MethodGet, "/metrics", nil, providerAuth)
	if status != http.StatusForbidden {
		t.Fatalf("non-admin provider reached management metrics: %d", status)
	}

	writerPath := "/writer/v1/repositories/" + url.PathEscape(repositoryID) + "/commits"
	schemaReceipt := liveServiceOK(t, server, writerPath, liveCommitRequest(repositoryID, "schema-"+run, "", knowledge.Operation{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/runbook.body"},
		Value: map[string]any{"entity": "Runbook", "pattern": "record", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}}},
	}), providerAuth)
	schemaCommit := receiptCommit(t, schemaReceipt)
	firstReceipt := liveServiceOK(t, server, writerPath, liveCommitRequest(repositoryID, "source-1-"+run, schemaCommit, knowledge.Operation{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "runbook/payment-oncall"},
		SchemaRef: "schema/runbook.body", Value: map[string]any{"body": "切换支付流量前先检查冻结窗口"},
	}), providerAuth)
	firstCommit := receiptCommit(t, firstReceipt)
	liveServiceOK(t, server, "/operations/v1/projections:sync", map[string]any{"repository": repositoryID, "commit": firstCommit}, providerAuth)

	catalogPath := "/catalog/v1/catalogs/" + url.PathEscape(catalogID)
	pin := asMap(t, liveServiceOK(t, server, catalogPath+"/workspaces/"+url.PathEscape(workspaceID)+"/resolve", map[string]any{}, consumerAuth))
	if asMap(t, pin["repositories"])[repositoryID] != firstCommit {
		t.Fatalf("consumer pin did not freeze first publish: %#v", pin)
	}
	pinJSON, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	search := asMap(t, liveServiceOK(t, server, "/knowledge/v1/search", map[string]any{
		"catalog": catalogID, "workspace": workspaceID, "pin": json.RawMessage(pinJSON), "query": "冻结窗口",
	}, consumerAuth))
	if search["completeness"] != "complete" || len(search["hits"].([]any)) != 1 {
		t.Fatalf("consumer search: %#v", search)
	}
	values := liveServiceOK(t, server, "/knowledge/v1/objects:read", map[string]any{
		"catalog": catalogID, "workspace": workspaceID, "pin": json.RawMessage(pinJSON), "object": "runbook/payment-oncall",
	}, consumerAuth).([]any)
	if len(values) != 1 || asMap(t, values[0])["commit"] != firstCommit {
		t.Fatalf("consumer read: %#v", values)
	}

	secondReceipt := liveServiceOK(t, server, writerPath, liveCommitRequest(repositoryID, "source-2-"+run, firstCommit, knowledge.Operation{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "runbook/payment-oncall"},
		SchemaRef: "schema/runbook.body", Value: map[string]any{"body": "新流程：切换前同时检查冻结窗口和容量水位"},
	}), providerAuth)
	secondCommit := receiptCommit(t, secondReceipt)
	liveServiceOK(t, server, "/operations/v1/projections:sync", map[string]any{"repository": repositoryID, "commit": secondCommit}, providerAuth)
	newPin := asMap(t, liveServiceOK(t, server, catalogPath+"/workspaces/"+url.PathEscape(workspaceID)+"/resolve", map[string]any{}, consumerAuth))
	if asMap(t, newPin["repositories"])[repositoryID] != secondCommit || secondCommit == firstCommit {
		t.Fatalf("new resolve did not advance: old=%s new=%#v", firstCommit, newPin)
	}
	oldSearch := asMap(t, liveServiceOK(t, server, "/knowledge/v1/search", map[string]any{
		"catalog": catalogID, "workspace": workspaceID, "pin": json.RawMessage(pinJSON), "query": "容量水位",
	}, consumerAuth))
	if len(oldSearch["hits"].([]any)) != 0 {
		t.Fatalf("old pin observed the new publish: %#v", oldSearch)
	}

	access, err := os.ReadFile(filepath.Join(home, "access.jsonl"))
	if err != nil || !bytes.Contains(access, []byte(fmt.Sprintf("gitea:%d", consumerID))) {
		t.Fatalf("consumer access evidence missing: %v %s", err, access)
	}
}

func liveCommitRequest(repositoryID, commandID, expected string, operation knowledge.Operation) map[string]any {
	return map[string]any{
		"commandId": commandID,
		"changeSet": knowledge.ChangeSet{
			TargetRepository: kernel.RepositoryID(repositoryID), TargetRef: snapshot.DefaultRef,
			BaseCommit: kernel.CommitID(expected), ExpectedTargetCommit: kernel.CommitID(expected),
			Operations: []knowledge.Operation{operation},
			Provenance: &knowledge.ProvenanceEnvelope{OriginKind: knowledge.OriginSource, SourceRefs: []string{"file:///source/runbooks/payment-oncall.md"}},
		},
	}
}

func createLiveGiteaUser(t *testing.T, base, adminToken, login, password string) int64 {
	t.Helper()
	payload := map[string]any{
		"username": login, "email": login + "@example.com", "password": password,
		"must_change_password": false, "send_notify": false,
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/admin/users", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create Gitea user %s: status=%d body=%s", login, res.StatusCode, raw)
	}
	var user map[string]any
	if err := json.Unmarshal(raw, &user); err != nil {
		t.Fatal(err)
	}
	return int64(user["id"].(float64))
}

func basicAuthorization(login, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(login+":"+password))
}

func liveServiceRequest(t *testing.T, server *httptest.Server, method, path string, body any, authorization string) (int, any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, server.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	res, err := server.Client().Do(req)
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
			t.Fatalf("decode live service response: %v: %s", err, raw)
		}
	}
	return res.StatusCode, payload
}

func liveServiceOK(t *testing.T, server *httptest.Server, path string, body any, authorization string) any {
	t.Helper()
	status, payload := liveServiceRequest(t, server, http.MethodPost, path, body, authorization)
	if status != http.StatusOK {
		t.Fatalf("%s status=%d body=%#v", path, status, payload)
	}
	return payload
}

func receiptCommit(t *testing.T, payload any) string {
	t.Helper()
	result := asMap(t, asMap(t, payload)["result"])
	commit, _ := result["newCommit"].(string)
	if commit == "" {
		t.Fatalf("receipt has no newCommit: %#v", payload)
	}
	return commit
}
