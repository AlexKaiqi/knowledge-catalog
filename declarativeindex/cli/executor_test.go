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
func (c *recordingClient) NotifyProjection(_ context.Context, r client.ProjectionNotifyRequest, _ client.RequestOptions, out any) error {
	return c.record(r, out)
}

func TestProjectionTaskHelpIsLocalAndAffordanceLinksExistingCommands(t *testing.T) {
	for _, entry := range Commands() {
		output, err := New(nil).Execute(context.Background(), entry.Path, map[string]any{"help": true}, client.RequestOptions{})
		if err != nil || !strings.Contains(output.(string), "后果：") || !strings.Contains(output.(string), "失败恢复：") {
			t.Fatalf("local maintenance help %s: %#v err=%v", entry.Path, output, err)
		}
	}
	affordance := Affordance()
	if affordance.Name != "declarative-index" || !strings.Contains(affordance.Usage, "同一依据") || !strings.Contains(affordance.Usage, "不现场建索引") {
		t.Fatalf("plugin guarantee/navigation missing: %#v", affordance)
	}
	for _, entry := range Commands() {
		found := false
		for _, task := range affordance.Tasks {
			if task == entry.Path {
				found = true
			}
		}
		if !found {
			t.Fatalf("plugin description omits maintenance task %s", entry.Path)
		}
	}
}

func TestProjectionCLITransportsCoordinatesAndPreservesActualResult(t *testing.T) {
	output := map[string]any{"snapshot": map[string]any{"basisCommit": "old"}, "state": map[string]any{"revision": "observed"}}
	remote := &recordingClient{output: output}
	e := New(remote)
	got, err := e.Execute(context.Background(), "operations projection sync", map[string]any{"repo": "kr://chosen", "commit": "old"}, client.RequestOptions{})
	if err != nil || !reflect.DeepEqual(Shape("operations projection sync", got), output) {
		t.Fatalf("sync result changed: %#v err=%v", got, err)
	}
	r := remote.request.(client.ProjectionRequest)
	if r.Repository != "kr://chosen" || r.Commit != "old" || r.Ref != "" {
		t.Fatalf("sync basis changed: %#v", r)
	}
	if _, err := e.Execute(context.Background(), "operations projection notice", map[string]any{"repo": "kr://chosen", "object": "job/orders", "aspect": "runtime", "source-revision": "rev-2"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	n := remote.request.(client.ProjectionNotifyRequest)
	if n.Address == nil || n.Address.Kind != "Aspect" || n.Address.ObjectID != "job/orders" || n.Address.AspectName != "runtime" || n.SourceRevision != "rev-2" {
		t.Fatalf("notice coordinates changed: %#v", n)
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
