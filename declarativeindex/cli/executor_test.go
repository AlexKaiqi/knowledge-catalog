package cli

import (
	"context"
	"kc/declarativeindex/client"
	"kc/kernel"
	"reflect"
	"strings"
	"testing"
)

type recordingClient struct {
	calls   int
	path    string
	request any
	output  any
}

func (c *recordingClient) record(path string, request any, out any) error {
	c.calls++
	c.path = path
	c.request = request
	*(out.(*any)) = c.output
	return nil
}
func (c *recordingClient) IndexReady(_ context.Context, r client.IndexRequest, _ client.RequestOptions, out any) error {
	return c.record("ready", r, out)
}
func (c *recordingClient) IndexPrepare(_ context.Context, r client.IndexRequest, _ client.RequestOptions, out any) error {
	return c.record("prepare", r, out)
}
func (c *recordingClient) IndexDescribe(_ context.Context, r client.IndexRequest, _ client.RequestOptions, out any) error {
	return c.record("describe", r, out)
}
func (c *recordingClient) IndexStatus(_ context.Context, _ client.RequestOptions, out any) error {
	return c.record("status", nil, out)
}

func TestIndexTaskHelpIsLocal(t *testing.T) {
	for _, entry := range Commands() {
		output, err := New(nil).Execute(context.Background(), entry.Path, map[string]any{"help": true}, client.RequestOptions{})
		if err != nil || !strings.Contains(output.(string), "后果：") || !strings.Contains(output.(string), "失败恢复：") {
			t.Fatalf("local maintenance help %s: %#v err=%v", entry.Path, output, err)
		}
	}
}

// Each index path answers a different question: `index` reads readiness of a
// named revision, `index prepare` builds, `index describe` attributes. The CLI
// must not collapse them into one call, and a not-ready answer must survive as
// the actual result rather than being rewritten into an error.
func TestIndexCLITransportsCoordinatesAndPreservesActualResult(t *testing.T) {
	output := map[string]any{"dataset": "chosen", "revision": 2, "ready": false}
	remote := &recordingClient{output: output}
	e := New(remote)
	cases := []struct {
		path string
		call string
	}{
		{"index", "ready"},
		{"index prepare", "prepare"},
		{"index describe", "describe"},
	}
	for _, tc := range cases {
		got, err := e.Execute(context.Background(), tc.path, map[string]any{"dataset": "chosen", "revision": "2"}, client.RequestOptions{})
		if err != nil || !reflect.DeepEqual(Shape(tc.path, got), output) {
			t.Fatalf("%s result changed: %#v err=%v", tc.path, got, err)
		}
		if remote.path != tc.call {
			t.Fatalf("%s must call %s, called %s", tc.path, tc.call, remote.path)
		}
		r := remote.request.(client.IndexRequest)
		if r.Dataset != "chosen" || r.Revision != 2 {
			t.Fatalf("%s basis changed: %#v", tc.path, r)
		}
	}
}

// Catch-up progress names no Dataset. That is the one path where a missing
// --dataset is legal; every other path still requires it.
func TestIndexWithoutDatasetReportsProgress(t *testing.T) {
	output := []any{map[string]any{"repository": "kr://r", "done": 3, "total": 9}}
	remote := &recordingClient{output: output}
	e := New(remote)
	got, err := e.Execute(context.Background(), "index", map[string]any{}, client.RequestOptions{})
	if err != nil || !reflect.DeepEqual(Shape("index", got), output) {
		t.Fatalf("progress result changed: %#v err=%v", got, err)
	}
	if remote.path != "status" || remote.request != nil {
		t.Fatalf("progress must be a scope-free call: %s %#v", remote.path, remote.request)
	}
	if _, err := e.Execute(context.Background(), "index prepare", map[string]any{}, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("prepare without --dataset accepted: %v", err)
	}
	if _, err := e.Execute(context.Background(), "index describe", map[string]any{}, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("describe without --dataset accepted: %v", err)
	}
	// A revision without a Dataset would name a coordinate the call cannot read.
	if _, err := e.Execute(context.Background(), "index", map[string]any{"revision": "2"}, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("--revision without --dataset accepted: %v", err)
	}
}

func TestIndexCLIRejectsForeignScopeAndBody(t *testing.T) {
	remote := &recordingClient{output: map[string]any{}}
	e := New(remote)
	for _, flags := range []map[string]any{{"repo": "r", "value": "unsafe-body"}, {"repo": "r", "dataset": "other"}, {"repo": true}} {
		if _, err := e.Execute(context.Background(), "index prepare", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid maintenance scope/body accepted: %#v err=%v", flags, err)
		}
	}
	for _, flags := range []map[string]any{{"dataset": "chosen", "revision": "0"}, {"dataset": "chosen", "revision": "latest"}} {
		if _, err := e.Execute(context.Background(), "index", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("non-positive revision accepted: %#v err=%v", flags, err)
		}
	}
	if remote.calls != 0 {
		t.Fatal("invalid maintenance request reached transport")
	}
}
