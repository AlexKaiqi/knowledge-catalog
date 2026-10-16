package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/access"
)

func TestNativeRepositoryMetadataPreservesAuthorityPayloadAndProof(t *testing.T) {
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryMetadataRead, access.RepositoryMetadataWrite}}}}
	metadata := map[string]string{"description": "old", "retained": "yes"}
	var bodies, proofs []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repositories/alpha/metadata" {
			t.Errorf("metadata used a different resource: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		proofs = append(proofs, r.Header.Get("Authorization"))
		w.Header().Set("X-Native-Metadata", "authority")
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, metadata)
		case http.MethodPost:
			var body struct {
				Metadata map[string]string `json:"metadata"`
			}
			if err := json.Unmarshal(raw, &body); err != nil || body.Metadata == nil {
				t.Errorf("native set payload invalid: %s %v", raw, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for key, value := range body.Metadata {
				metadata[key] = value
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			var body struct {
				Keys []string `json:"keys"`
			}
			if err := json.Unmarshal(raw, &body); err != nil || body.Keys == nil {
				t.Errorf("native delete payload invalid: %s %v", raw, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, key := range body.Keys {
				delete(metadata, key)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(backend.Close)
	gateway := NewHandler(Config{NativeOrigin: backend.URL, NativeCredential: func(context.Context) (string, error) { return nativeServiceCredential, nil }, Identity: IdentityOptions{Authenticator: basicVerifier}, Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
		return Resources{NativeRepositories: map[string]kernel.RepositoryID{"alpha": "kr://test/a"}, Access: access.NewService(rules)}, func() {}, nil
	}})
	path := "/api/v1/repositories/alpha/metadata"
	set := `{"metadata":{"description":"新用途","source_repository":"beta","source":"private","owner":"bob","custom":""}}`
	callNativeTest(t, gateway, "alice", "POST", path, set, 204)
	callNativeTest(t, gateway, "bob", "GET", path, "", 403)
	callNativeTest(t, gateway, "bob", "POST", path, `{"metadata":{"description":"owner field is not authority"}}`, 403)
	response := callNativeTest(t, gateway, "alice", "GET", path, "", 200)
	var got map[string]string
	if json.Unmarshal(response.Body.Bytes(), &got) != nil || !reflect.DeepEqual(got, metadata) || got["retained"] != "yes" || got["description"] != "新用途" {
		t.Fatalf("native incremental metadata lost: %s", response.Body.String())
	}
	if response.Header().Get("X-Native-Metadata") != "authority" {
		t.Fatal("native metadata response headers changed")
	}
	remove := `{"keys":["custom","source"]}`
	callNativeTest(t, gateway, "alice", "DELETE", path, remove, 204)
	metadata["description"] = "changed directly in native authority"
	response = callNativeTest(t, gateway, "alice", "GET", path, "", 200)
	if json.Unmarshal(response.Body.Bytes(), &got) != nil || got["description"] != metadata["description"] || strings.Contains(response.Body.String(), `"custom"`) || strings.Contains(response.Body.String(), `"source"`) {
		t.Fatalf("native authority changed or delete became replacement: %s", response.Body.String())
	}
	if !reflect.DeepEqual(bodies, []string{set, "", remove, ""}) {
		t.Fatalf("metadata payload altered or extra native reads/writes: %#v", bodies)
	}
	for _, proof := range proofs {
		if proof != nativeServiceProof() {
			t.Fatalf("metadata request forwarded caller proof instead of the service credential: %q", proof)
		}
	}
}

func TestNativeRepositoryMetadataReadAndWriteRemainSeparate(t *testing.T) {
	for _, row := range []struct {
		actions []string
		read    int
		write   int
	}{
		{[]string{access.RepositoryMetadataRead}, 200, 403},
		{[]string{access.RepositoryMetadataWrite}, 403, 201},
		{[]string{access.RepositorySettingsManage, access.RepositoryFilesWrite}, 403, 403},
	} {
		rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: row.actions}}}
		handler, upstream := nativeTestHandler(t, rules)
		path := "/api/v1/repositories/alpha/metadata"
		callNativeTest(t, handler, "alice", "GET", path, "", row.read)
		callNativeTest(t, handler, "alice", "POST", path, `{"metadata":{"description":"value"}}`, row.write)
		deleteStatus := row.write
		if deleteStatus == 201 {
			deleteStatus = 200
		}
		callNativeTest(t, handler, "alice", "DELETE", path, `{"keys":["description"]}`, deleteStatus)
		count := 0
		if row.read == 200 {
			count++
		}
		if row.write == 201 {
			count += 2
		}
		if len(upstream.calls) != count {
			t.Fatalf("denied metadata request forwarded: %#v", upstream.calls)
		}
		callNativeTest(t, handler, "bob", "POST", path, `{"metadata":{"description":"other principal"}}`, 403)
		callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/beta/metadata", `{"metadata":{"description":"other repository"}}`, 403)
		callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/unregistered/metadata", `{"metadata":{"description":"unregistered"}}`, 403)
		callNativeTest(t, handler, "alice", "PUT", path, `{"metadata":{}}`, 403)
		if len(upstream.calls) != count {
			t.Fatal("different principal, repository or method widened metadata grant")
		}
	}
}

func TestNativeRepositoryMetadataQualifiedGrantsAndRevocationFailClosed(t *testing.T) {
	for _, rule := range []access.Rule{{Dataset: "data"}, {Object: "*"}, {Aspect: "metadata"}, {Ref: "main"}} {
		rule.Principal, rule.Repo = "alice", "kr://test/a"
		rule.Actions = []string{access.RepositoryMetadataRead, access.RepositoryMetadataWrite}
		handler, upstream := nativeTestHandler(t, &nativeTestRules{rules: []access.Rule{rule}})
		callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/alpha/metadata", `{"metadata":{"owner":"alice"}}`, 403)
		callNativeTest(t, handler, "alice", "GET", "/api/v1/repositories/alpha/metadata", "", 403)
		if len(upstream.calls) != 0 {
			t.Fatal("qualified grant became repository metadata permission")
		}
	}
	rules := &nativeTestRules{rules: []access.Rule{{Principal: "alice", Repo: "kr://test/a", Actions: []string{access.RepositoryMetadataWrite}}}}
	handler, upstream := nativeTestHandler(t, rules)
	callNativeTest(t, handler, "alice", "POST", "/api/v1/repositories/alpha/metadata", `{"metadata":{"description":"before revoke"}}`, 201)
	if err := rules.UpdateRules(context.Background(), func([]access.Rule) ([]access.Rule, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	callNativeTest(t, handler, "alice", "DELETE", "/api/v1/repositories/alpha/metadata", `{"keys":["description"]}`, 403)
	if len(upstream.calls) != 1 {
		t.Fatal("revoked metadata write was forwarded")
	}
}
