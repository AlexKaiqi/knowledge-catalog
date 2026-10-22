package cli

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/object/client"
)

type comparisonFakeClient struct {
	client.Client
	request client.ComparisonRequest
	calls   int
}

func (c *comparisonFakeClient) Compare(_ context.Context, r client.ComparisonRequest, _ client.RequestOptions, out any) error {
	c.calls++
	c.request = r
	*(out.(*any)) = map[string]any{"equal": true}
	return nil
}

func TestComparisonCLIHasIndependentViewsAndDoesNotInheritDataset(t *testing.T) {
	remote := &comparisonFakeClient{}
	executor := New(remote, Context{Dataset: "inherited"})
	flags := map[string]any{"from": `{"basis":"saved"}`, "to": `{"dataset":"published","revision":2}`}
	if err := BindOperands("diff", flags, []string{testReference}); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), "diff", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.calls != 1 || remote.request.From.Basis != "saved" || remote.request.From.Dataset != "" || remote.request.To.Dataset != "published" || remote.request.To.Revision != 2 || remote.request.Reference != testReference {
		t.Fatalf("comparison borrowed a default or rewrote target: %#v", remote)
	}
	for _, raw := range []string{`{}`, `null`, `{"dataset":"published","revision":0}`, `{"basis":"saved","commit":"c1","repository":"r"}`, `{"basis":"saved","ref":"main"}`, `{"basis":"saved"} {}`} {
		flags["to"] = raw
		if _, err := executor.Execute(context.Background(), "diff", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid || remote.calls != 1 {
			t.Fatalf("invalid second side reached transport: %s %v calls=%d", raw, err, remote.calls)
		}
	}
}
