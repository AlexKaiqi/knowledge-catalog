package cli

import (
	"context"
	"kc/kernel"
	"kc/object/client"
	"testing"
)

type schemaClient struct {
	fakeClient
	page   client.SchemaPageRequest
	schema client.SchemaRequest
}

func (c *schemaClient) BrowseSchemas(_ context.Context, r client.SchemaPageRequest, _ client.RequestOptions, out any) error {
	c.page = r
	*(out.(*any)) = map[string]any{}
	return nil
}
func (c *schemaClient) Schema(_ context.Context, r client.SchemaRequest, _ client.RequestOptions, out any) error {
	c.schema = r
	*(out.(*any)) = []any{}
	return nil
}

func TestDatasetRevisionReachesSchemaAndReadRequests(t *testing.T) {
	remote := &schemaClient{}
	e := New(remote, Context{})
	if _, err := e.Execute(context.Background(), "schema list", map[string]any{"dataset": "set", "revision": "3", "limit": "1"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.page.Dataset != "set" || remote.page.Revision != 3 || remote.page.Repository != "" || remote.page.Limit != 1 {
		t.Fatalf("schema list --dataset lost its scope: %#v", remote.page)
	}
	if _, err := e.Execute(context.Background(), "schema describe", map[string]any{"dataset": "set", "revision": "3", "object": "schema/x"}, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.schema.Dataset != "set" || remote.schema.Revision != 3 {
		t.Fatalf("schema describe lost the revision: %#v", remote.schema)
	}
	flags := positionalReferenceFlags(t, "read", testReference, map[string]any{"dataset": "set", "revision": "3"})
	if _, err := e.Execute(context.Background(), "read", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.read.Dataset != "set" || remote.read.Revision != 3 {
		t.Fatalf("read lost the revision: %#v", remote.read)
	}
}

func TestDatasetRevisionRequiresNamedDatasetAndPositiveValue(t *testing.T) {
	e := New(&schemaClient{}, Context{})
	for _, flags := range []map[string]any{
		{"repo": "kr://r", "revision": "1"},
		{"revision": "1"},
		{"dataset": "set", "revision": "0"},
		{"dataset": "set", "revision": "x"},
	} {
		if err := e.Validate("schema list", flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid revision accepted: %#v err=%v", flags, err)
		}
	}
}
