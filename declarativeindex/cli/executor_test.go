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
	request any
	output  any
}

func (c *recordingClient) record(request any, out any) error {
	c.calls++
	c.request = request
	*(out.(*any)) = c.output
	return nil
}
func (c *recordingClient) DescribeProjection(_ context.Context, r client.ProjectionRequest, _ client.RequestOptions, out any) error {
	return c.record(r, out)
}
func (c *recordingClient) SyncProjection(_ context.Context, r client.ProjectionRequest, _ client.RequestOptions, out any) error {
	return c.record(r, out)
}

func TestProjectionTaskHelpIsLocal(t *testing.T) {
	for _, entry := range Commands() {
		output, err := New(nil).Execute(context.Background(), entry.Path, map[string]any{"help": true}, client.RequestOptions{})
		if err != nil || !strings.Contains(output.(string), "后果：") || !strings.Contains(output.(string), "失败恢复：") {
			t.Fatalf("local maintenance help %s: %#v err=%v", entry.Path, output, err)
		}
	}
}

func TestProjectionCLITransportsCoordinatesAndPreservesActualResult(t *testing.T) {
	output := map[string]any{"snapshot": map[string]any{"basisCommit": "old"}, "state": map[string]any{"revision": "observed"}}
	remote := &recordingClient{output: output}
	e := New(remote)
	got, err := e.Execute(context.Background(), "operations projection sync", map[string]any{"dataset": "chosen", "revision": "2"}, client.RequestOptions{})
	if err != nil || !reflect.DeepEqual(Shape("operations projection sync", got), output) {
		t.Fatalf("sync result changed: %#v err=%v", got, err)
	}
	r := remote.request.(client.ProjectionRequest)
	if r.Dataset != "chosen" || r.Revision != 2 {
		t.Fatalf("sync basis changed: %#v", r)
	}
	before := remote.calls
	for _, flags := range []map[string]any{{"repo": "r", "value": "unsafe-body"}, {"repo": "r", "dataset": "other"}, {"repo": true}} {
		if _, err := e.Execute(context.Background(), "operations projection sync", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid maintenance scope/body accepted: %#v err=%v", flags, err)
		}
	}
	if remote.calls != before {
		t.Fatal("invalid maintenance request reached transport")
	}
}
