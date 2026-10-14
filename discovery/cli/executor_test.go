package cli

import (
	"context"
	"encoding/json"
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

func TestDiscoveryCLIUsesHostFixedContextAndKeepsRepositoryRequestsSeparate(t *testing.T) {
	remote := &recordingClient{}
	pin := json.RawMessage(`{"setId":"fixed","repositories":{"kr://source":"old"}}`)
	e := New(remote, Context{Dataset: "fixed", Pin: pin, InventoryDiscovery: true})
	if _, err := e.Execute(context.Background(), "search", map[string]any{"query": "runbook", "eq": []string{"number=9007199254740993"}, "limit": "10", "continuation": "cursor"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	r := remote.request.(client.SearchRequest)
	if r.Dataset != "fixed" || !reflect.DeepEqual(r.Pin, pin) || r.Continuation != "cursor" || r.Equal[0] != "number=9007199254740993" || !r.InventoryDiscovery {
		t.Fatalf("fixed caller scope/operators changed: %#v", r)
	}
	if _, err := e.Execute(context.Background(), "search", map[string]any{"repo": "kr://direct", "commit": "c1", "query": "runbook"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	r = remote.request.(client.SearchRequest)
	if r.Repository != "kr://direct" || r.Commit != "c1" || r.Dataset != "" || len(r.Pin) != 0 || r.Definition != nil || r.InventoryDiscovery {
		t.Fatalf("Dataset context contaminated direct repository: %#v", r)
	}
	for _, flags := range []map[string]any{{"repo": "r", "dataset": "d"}, {"dataset": "d", "commit": "c1"}} {
		if err := e.Validate("search", flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("mixed scope accepted: %#v err=%v", flags, err)
		}
	}
	if _, err := e.Execute(context.Background(), "search", map[string]any{"repo": "r", "commit": "c1", "ref": "release", "query": "runbook"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	both := remote.request.(client.SearchRequest)
	if both.Commit != "c1" || both.Ref != "release" {
		t.Fatalf("existing explicit commit/ref request changed: %#v", both)
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
