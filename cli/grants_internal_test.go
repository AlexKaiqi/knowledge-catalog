package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kcclient "kc/client"
	inv "kc/invocation"
	"kc/kernel"
)

func TestRemoteGrantListPreservesInventoryFiltersAndLegacyExplicitQuestion(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/admin/v1/grants" || r.Header.Get("X-Kc-As") != "operator" {
			t.Errorf("wrong typed grant request: %s %s %v", r.Method, r.URL.RequestURI(), r.Header)
		}
		query = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(inv.GrantList{Rules: []inv.AllowRule{{ID: "limited", Principal: "reader", Repo: "repo-A", Object: "data/*", Actions: []string{"knowledge.read"}}}})
	}))
	t.Cleanup(server.Close)
	client, err := kcclient.New(kcclient.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "operator"}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]FlagValue{"repo": "repo-A", "principal": "reader", "ref": "main", "object": "data/*", "aspect": "body"}
	listed, err := runRemoteGrantList(context.Background(), client, flags, kcclient.RequestOptions{})
	if err != nil || len(listed.(inv.GrantList).Rules) != 1 || query != "aspect=body&object=data%2F%2A&principal=reader&ref=main&repository=repo-A" {
		t.Fatalf("remote inventory dropped filters: %#v %s %v", listed, query, err)
	}
	flags["action"] = "knowledge.read"
	flags["object"] = "data/file"
	value, err := runRemoteGrantList(context.Background(), client, flags, kcclient.RequestOptions{})
	if err != nil || value.(map[string]any)["ruleId"] != "limited" || query != "action=knowledge.read&principal=reader" {
		t.Fatalf("remote question became a list or excluded applicable global rules: %#v %s %v", value, query, err)
	}
	flags["repo"] = "repo-other"
	if _, err := runRemoteGrantList(context.Background(), client, flags, kcclient.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("legacy explicit-rule question widened target scope: %v", err)
	}
}

func TestCLIAdmissionPreservesEveryStoredConstraint(t *testing.T) {
	dir := t.TempDir()
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []inv.AllowRule{{ID: "limited", Principal: "reader", Dataset: "one", Ref: "main", Object: "data/*", Aspect: "body", Actions: []string{"file.read"}}}}); err != nil {
		t.Fatal(err)
	}
	value, err := admissionOverview(&inv.Invocation{Home: dir, Flags: map[string]FlagValue{"as": "reader"}})
	if err != nil || len(value.Grants) != 1 {
		t.Fatalf("admission: %#v %v", value, err)
	}
	grant := value.Grants[0]
	if grant.Dataset != "one" || grant.Ref != "main" || grant.Object != "data/*" || grant.Aspect != "body" {
		t.Fatalf("CLI admission widened stored grant: %#v", grant)
	}
}
