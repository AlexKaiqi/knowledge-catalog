package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(context.Context, string, *http.Request) (*http.Response, error)

func (f transportFunc) Do(ctx context.Context, audience string, request *http.Request) (*http.Response, error) {
	return f(ctx, audience, request)
}

func TestDiscoveryTransportKeepsExistingQueryWireAndScope(t *testing.T) {
	paths := []string{}
	remote, err := New(Config{BaseURL: "https://example.test/api", Transport: transportFunc(func(_ context.Context, audience string, request *http.Request) (*http.Response, error) {
		if audience != "https://example.test/api" || request.Method != "POST" || request.Header.Get("X-Kc-Request-Id") != "request-1" {
			t.Fatalf("authenticated transport context lost: audience=%s request=%#v", audience, request)
		}
		paths = append(paths, request.URL.Path)
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["repository"] != "kr://source" || body["commit"] != "fixed" || body["dataset"] != nil || body["pin"] != nil {
			t.Fatalf("repository scope changed: %#v", body)
		}
		if strings.HasSuffix(request.URL.Path, "/search") {
			if body["query"] != "orders" || body["matchMode"] != "Phrase" || body["continuation"] != "cursor" || body["recall"] != "lexical" {
				t.Fatalf("query controls changed: %#v", body)
			}
			for _, name := range []string{"match", "equal", "notEqual", "in", "exists", "missing", "prefix", "contains", "greaterThan", "greaterEqual", "lessThan", "lessEqual", "sort"} {
				if len(body[name].([]any)) != 1 {
					t.Fatalf("query operator %s lost: %#v", name, body)
				}
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"value":9007199254740993}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var output any
	o := RequestOptions{RequestID: "request-1"}
	scope := KnowledgeScope{Repository: "kr://source", Commit: "fixed"}
	err = remote.Search(context.Background(), SearchRequest{KnowledgeScope: scope, Query: "orders", MatchMode: "Phrase", Match: []string{"body=orders"}, Equal: []string{"count=9007199254740993"}, NotEqual: []string{"state=cancelled"}, In: []string{"state=open,closed"}, Exists: []string{"owner"}, Missing: []string{"archived"}, Prefix: []string{"code=ORD"}, Contains: []string{"name=order"}, GreaterThan: []string{"count=1"}, GreaterEqual: []string{"count=2"}, LessThan: []string{"count=10"}, LessEqual: []string{"count=9"}, Sort: []string{"count:asc"}, Continuation: "cursor", Recall: "lexical"}, o, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.(map[string]any)["value"] != json.Number("9007199254740993") {
		t.Fatalf("precise result changed: %#v", output)
	}
	if err := remote.Relations(context.Background(), RelationsRequest{KnowledgeScope: scope, Endpoint: "orders", Role: "from", Continuation: "relations-cursor"}, o, &output); err != nil {
		t.Fatal(err)
	}
	if err := remote.Traverse(context.Background(), TraverseRequest{KnowledgeScope: scope, Endpoint: "orders", MaxHops: 2}, o, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/api/knowledge/v1/search,/api/knowledge/v1/relations:query,/api/knowledge/v1/traverse:query" {
		t.Fatal(paths)
	}
}

func TestPluginHelpAndAccessOverviewTransportRemainReadOnly(t *testing.T) {
	paths := []string{}
	remote, err := New(Config{BaseURL: "https://example.test", Transport: transportFunc(func(_ context.Context, _ string, request *http.Request) (*http.Response, error) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		if request.Method == "GET" && request.Body != nil && request.Body != http.NoBody {
			t.Fatal("plugin description acquired a target/body")
		}
		if request.Method == "POST" {
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["dataset"] != "chosen" || body["repository"] != nil || body["pin"].(map[string]any)["setId"] != "chosen" {
				t.Fatalf("overview fixed scope lost: %#v", body)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"access":{"members":[]},"plugins":[{"repository":"kr://chosen","name":"declarative-index"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	var output any
	if err := remote.PluginHelp(context.Background(), "experimental/name", RequestOptions{}, &output); err != nil {
		t.Fatal(err)
	}
	if err := remote.DescribeAccessSpec(context.Background(), AccessSpecDescribeRequest{Dataset: "chosen", Pin: json.RawMessage(`{"setId":"chosen"}`)}, RequestOptions{}, &output); err != nil {
		t.Fatal(err)
	}
	if output.(map[string]any)["plugins"].([]any)[0].(map[string]any)["repository"] != "kr://chosen" {
		t.Fatalf("source capability lost: %#v", output)
	}
	if strings.Join(paths, ",") != "GET /knowledge/v1/plugins/experimental%2Fname,POST /operations/v1/access-specs:describe" {
		t.Fatal(paths)
	}
}
