package cli

import (
	"context"
	"encoding/json"
	retrieval "kc/discovery"
	"kc/discovery/client"
	"kc/kernel"
	"reflect"
	"strings"
	"testing"
)

type recordingClient struct {
	calls      int
	path, name string
	request    any
	output     any
}

func (c *recordingClient) record(path string, request any, out any) error {
	c.calls++
	c.path = path
	c.request = request
	*(out.(*any)) = c.output
	return nil
}
func (c *recordingClient) Search(_ context.Context, r client.SearchRequest, _ client.RequestOptions, out any) error {
	return c.record("search", r, out)
}
func (c *recordingClient) Relations(_ context.Context, r client.RelationsRequest, _ client.RequestOptions, out any) error {
	return c.record("relations", r, out)
}
func (c *recordingClient) Traverse(_ context.Context, r client.TraverseRequest, _ client.RequestOptions, out any) error {
	return c.record("traverse", r, out)
}
func (c *recordingClient) DescribeAccessSpec(_ context.Context, r client.AccessSpecDescribeRequest, _ client.RequestOptions, out any) error {
	return c.record("overview", r, out)
}
func (c *recordingClient) PluginHelp(_ context.Context, name string, _ client.RequestOptions, out any) error {
	c.name = name
	return c.record("plugin", nil, out)
}
func (c *recordingClient) PluginCapabilities(_ context.Context, name string, _ client.RequestOptions, out any) error {
	c.name = name
	return c.record("capabilities", nil, out)
}
func (c *recordingClient) TrialDeclarations(_ context.Context, name string, request retrieval.DeclarationTrialRequest, _ client.RequestOptions, out any) error {
	c.name = name
	return c.record("trial", request, out)
}

func TestDiscoveryCLIHelpIsLocalAndCommandDeclarationsAreIndependent(t *testing.T) {
	for _, entry := range Commands() {
		output, err := New(nil, Context{}).Execute(context.Background(), entry.Path, map[string]any{"help": true}, client.RequestOptions{})
		if err != nil || !strings.Contains(output.(string), "条件：") || !strings.Contains(output.(string), "失败恢复：") {
			t.Fatalf("module local help %s: %#v err=%v", entry.Path, output, err)
		}
	}
	entries := Commands()
	entries[0].Flags[0] = "changed"
	if Commands()[0].Flags[0] == "changed" {
		t.Fatal("host mutated module command contract")
	}
}

func TestPluginCLIRejectsNonHelpTasksAndTargetsBeforeTransport(t *testing.T) {
	remote := &recordingClient{output: map[string]any{"name": "custom-plugin", "usage": "use existing tasks"}}
	e := New(remote, Context{Dataset: "ignored-default"})
	for _, flags := range []map[string]any{
		{"name": "custom-plugin", "task": "install"},
		{"task": "help"},
		{"name": "custom-plugin", "task": "help", "repo": "kr://secret"},
		{"name": "custom-plugin", "task": "help", "task-extra": "delete"},
		{"name": []string{"one", "two"}, "task": "help"},
		{"name": "custom-plugin", "task": []string{"install", "help"}},
	} {
		if _, err := e.Execute(context.Background(), "plugin", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid plugin operands reached execution: %#v err=%v", flags, err)
		}
	}
	if remote.calls != 0 {
		t.Fatalf("rejected request performed %d calls", remote.calls)
	}
	output, err := e.Execute(context.Background(), "plugin", map[string]any{"name": "custom-plugin", "task": "help"}, client.RequestOptions{})
	if err != nil || remote.name != "custom-plugin" || remote.calls != 1 || !reflect.DeepEqual(output, remote.output) {
		t.Fatalf("description did not use registered-name transport: %#v err=%v", output, err)
	}
}

func TestDiscoveryCLIUsesOnlyTheNamedDatasetAndKeepsItsFixedBasis(t *testing.T) {
	remote := &recordingClient{}
	pin := json.RawMessage(`{"setId":"fixed","repositories":{"kr://source":"old"}}`)
	e := New(remote, Context{Dataset: "fixed", Pin: pin})
	if _, err := e.Execute(context.Background(), "search", map[string]any{"query": "runbook", "eq": []string{"number=9007199254740993"}, "limit": "10", "continuation": "cursor"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	r := remote.request.(client.SearchRequest)
	if r.Dataset != "fixed" || !reflect.DeepEqual(r.Pin, pin) || r.Continuation != "cursor" || r.Equal[0] != "number=9007199254740993" {
		t.Fatalf("fixed Dataset basis/operators changed: %#v", r)
	}
	for _, path := range []string{"search", "relations", "traverse"} {
		for _, scope := range []map[string]any{{"repo": "kr://direct"}, {"dataset": "fixed", "repo": "kr://direct"}, {"dataset": "fixed", "commit": "c1"}, {"dataset": "fixed", "ref": "release"}} {
			flags := map[string]any{"query": "runbook", "object": "a/b", "max-hops": "1"}
			if path == "search" {
				delete(flags, "object")
				delete(flags, "max-hops")
			} else {
				delete(flags, "query")
				if path == "relations" {
					delete(flags, "max-hops")
				}
			}
			for name, value := range scope {
				flags[name] = value
			}
			before := remote.calls
			_, err := e.Execute(context.Background(), path, flags, client.RequestOptions{})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid || remote.calls != before {
				t.Fatalf("retired scope reached %s transport: %#v calls=%d err=%v", path, flags, remote.calls, err)
			}
		}
	}
}

func TestDiscoveryCLIRejectsImplicitAndTemporarySearchScopesBeforeTransport(t *testing.T) {
	remote := &recordingClient{}
	for _, ctx := range []Context{{}, {Pin: json.RawMessage(`{"setId":"forged"}`)}} {
		for _, path := range []string{"search", "relations", "traverse"} {
			flags := map[string]any{}
			if path == "search" {
				flags["query"] = "q"
			} else {
				flags["object"] = "a/b"
				if path == "traverse" {
					flags["max-hops"] = "1"
				}
			}
			_, err := New(remote, ctx).Execute(context.Background(), path, flags, client.RequestOptions{})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("implicit scope reached %s: %#v err=%v", path, ctx, err)
			}
		}
	}
	if remote.calls != 0 {
		t.Fatalf("rejected implicit scopes performed %d HTTP requests", remote.calls)
	}
}

func TestDiscoveryProductViewsPreserveScopeAndKeepOverviewFacts(t *testing.T) {
	view := map[string]any{"snapshots": map[string]any{"r": "c1"}}
	search := map[string]any{"searchView": view, "completeness": "partial", "claims": []any{"budget"}, "continuation": "next", "hits": []any{map[string]any{"object": map[string]any{"repository": "r", "commit": "c1", "address": map[string]any{"objectId": "policy/P-1"}, "value": "protected"}}}}
	got := Shape("search", search).(map[string]any)
	if !reflect.DeepEqual(got["searchView"], view) || got["completeness"] != "partial" || got["continuation"] != "next" {
		t.Fatalf("product view lost query guarantees: %#v", got)
	}
	hit := got["hits"].([]any)[0].(map[string]any)
	if hit["objectId"] != "policy/P-1" || hit["repository"] != "r" || hit["commit"] != "c1" || hit["value"] != nil {
		t.Fatalf("query product coordinates changed: %#v", hit)
	}
	overview := map[string]any{"members": []any{}, "plugins": []any{map[string]any{"repository": "r", "name": "declarative-index", "help": "kc plugin declarative-index help"}}}
	if !reflect.DeepEqual(Shape("operations access-spec describe", overview), overview) {
		t.Fatal("overview encoder discarded existing or sourced fields")
	}
}
