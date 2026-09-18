package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent(t *testing.T) {
	home := t.TempDir()
	mustKnowledgeSetFSRun(t, home, "init", "--catalog", "kr://acme/catalog")
	server := httptest.NewServer(HTTPHandler(home))
	defer server.Close()

	request := func(payload map[string]any) (int, map[string]any, string) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		httpRequest, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/schemas:list", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("X-Kc-As", "reader")
		response, err := server.Client().Do(httpRequest)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.Unmarshal(responseBody, &body)
		return response.StatusCode, body, string(responseBody)
	}

	page := func(continuation string) map[string]any {
		t.Helper()
		payload := map[string]any{"repository": "kr://kc/system", "limit": 2}
		if continuation != "" {
			payload["continuation"] = continuation
		}
		status, body, raw := request(payload)
		if status != http.StatusOK {
			t.Fatalf("schema page returned %d: %s", status, raw)
		}
		return body
	}

	wantTotal := len(knowledge.SystemSchemaOperations())
	first := page("")
	if first["repository"] != "kr://kc/system" || first["continuation"] == nil {
		t.Fatalf("unexpected first page: %#v", first)
	}
	if schemas, ok := first["schemas"].([]any); !ok || len(schemas) != 2 {
		t.Fatalf("unexpected first schema page: %#v", first["schemas"])
	}
	row := first["schemas"].([]any)[0].(map[string]any)
	if row["repository"] != nil || row["commit"] != nil || row["fields"] != nil || row["digest"] != nil ||
		row["path"] != nil || row["body"] != nil {
		t.Fatalf("schema list must name entities, not return a schema document: %#v", row)
	}
	if row["objectId"] == "" || row["entity"] == "" || row["description"] == "" {
		t.Fatalf("schema list row missing entity name: %#v", row)
	}
	second := page(first["continuation"].(string))
	if second["continuation"] != nil || second["coverage"] != nil || second["exhausted"] != nil || second["total"] != nil {
		t.Fatalf("last page must omit pagination inventions: %#v", second)
	}

	status, zero, raw := request(map[string]any{"repository": "kr://kc/system", "limit": 0})
	if status != http.StatusOK || zero["continuation"] != nil || len(zero["schemas"].([]any)) != wantTotal {
		t.Fatalf("schema page limit 0 must mean the default page: status=%d payload=%#v raw=%s", status, zero, raw)
	}
	status, oversized, raw := request(map[string]any{"repository": "kr://kc/system", "limit": 201})
	errObj, _ := oversized["error"].(map[string]any)
	if status != http.StatusBadRequest || errObj["code"] != string(kernel.ErrUsageInvalid) {
		t.Fatalf("schema page limit 201 status=%d payload=%#v raw=%s", status, oversized, raw)
	}
}

type namedSchemaRepo struct{ knowledge.Repository }

func (namedSchemaRepo) ID() kernel.RepositoryID { return "kr://tables-only" }

func (namedSchemaRepo) Read(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	return knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Object: objectID},
		Value: map[string]any{
			"entity": "Note", "description": "A short note.", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
		},
	}, nil
}

func TestSchemaListNamesEntitiesWithoutContractDetails(t *testing.T) {
	listed, err := schemaEntityDirectory(namedSchemaRepo{}, "commit-1", []knowledge.ObjectID{"schema/note"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ObjectID != "schema/note" || listed[0].Entity != "Note" || listed[0].Description != "A short note." {
		t.Fatalf("schema list must name the entity: %#v", listed)
	}
}
