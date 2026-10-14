package cli

import (
	"context"
	"encoding/json"
	"kc/kernel"
	"kc/object/client"
	"reflect"
	"strings"
	"testing"
)

type fakeClient struct {
	client.Client
	read        client.ReadRequest
	put, remove client.MaintenanceRequest
	calls       []string
}

func (c *fakeClient) Read(_ context.Context, r client.ReadRequest, _ client.RequestOptions, out any) error {
	c.read = r
	c.calls = append(c.calls, "read")
	*(out.(*any)) = map[string]any{"value": false}
	return nil
}
func (c *fakeClient) Put(_ context.Context, r client.MaintenanceRequest, _ client.RequestOptions, out any) error {
	c.put = r
	c.calls = append(c.calls, "put")
	return nil
}
func (c *fakeClient) Remove(_ context.Context, r client.MaintenanceRequest, _ client.RequestOptions, out any) error {
	c.remove = r
	c.calls = append(c.calls, "remove")
	return nil
}

const testReference = "urn:kc:object:kr%3A%2F%2Fscene%2Fobject:table%2Forders#/columns/order_id/constraints/nullable"

func TestReferenceReadPreservesSelectionAndExplicitScope(t *testing.T) {
	remote := &fakeClient{}
	e := New(remote, Context{Dataset: "inherited"})
	_, err := e.Execute(context.Background(), "read", positionalReferenceFlags(t, "read", testReference, map[string]any{"commit": "c1"}), client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if remote.read.Reference != testReference || remote.read.Commit != "c1" || remote.read.Dataset != "" {
		t.Fatalf("reference was rewritten or inherited consumer scope: %#v", remote.read)
	}
	_, err = e.Execute(context.Background(), "read", positionalReferenceFlags(t, "read", testReference, map[string]any{"dataset": "delivery"}), client.RequestOptions{})
	if err != nil || remote.read.Dataset != "delivery" {
		t.Fatalf("explicit delivery scope: %#v %v", remote.read, err)
	}
	_, err = e.Execute(context.Background(), "read", positionalReferenceFlags(t, "read", testReference, map[string]any{"object": "other"}), client.RequestOptions{})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(remote.calls) != 2 {
		t.Fatalf("mixed identity reached transport: %v %#v", err, remote.calls)
	}
}

func TestReferenceWriteDoesNotGuessMaintenanceFromPointer(t *testing.T) {
	remote := &fakeClient{}
	e := New(remote, Context{})
	flags := positionalReferenceFlags(t, "writer put", testReference, map[string]any{"command-id": "put-1", "value": "null", "if-digest": "unit-digest", "kind": "Member"})
	if _, err := e.Execute(context.Background(), "writer put", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.put.Reference != testReference || remote.put.Value != nil || remote.put.IfDigest != "unit-digest" || remote.put.Kind != "Member" || !reflect.DeepEqual(remote.calls, []string{"put"}) {
		t.Fatalf("write scope/value/condition changed: %#v %#v", remote.put, remote.calls)
	}
	root := strings.Split(testReference, "#")[0]
	if _, err := e.Execute(context.Background(), "writer remove", positionalReferenceFlags(t, "writer remove", root, map[string]any{"command-id": "remove-1"}), client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.remove.Reference != root || !reflect.DeepEqual(remote.calls, []string{"put", "remove"}) {
		t.Fatalf("root delete was changed into another operation: %#v", remote.remove)
	}
}

func TestReferenceWriteRetainsUnitCASAndJSONPrecision(t *testing.T) {
	remote := &fakeClient{}
	reference := strings.Split(testReference, "/constraints")[0]
	flags := positionalReferenceFlags(t, "writer put", reference, map[string]any{"value": "{\"n\":9007199254740993}", "command-id": "put-precision", "base": "c1", "expected": "c1", "if-digest": "d1", "source-ref": []string{"a", "b"}})
	if _, err := New(remote, Context{}).Execute(context.Background(), "writer put", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	request := remote.put
	if request.Reference != reference || request.Base != "c1" || request.Expected != "c1" || request.IfDigest != "d1" || !reflect.DeepEqual(remote.calls, []string{"put"}) {
		t.Fatalf("reference or concurrency condition changed: %#v %#v", request, remote.calls)
	}
	if request.Value.(map[string]any)["n"] != json.Number("9007199254740993") || !reflect.DeepEqual(request.Provenance.SourceRefs, []string{"a", "b"}) {
		t.Fatalf("lost content/source: %#v", request)
	}
}

func TestBasicObjectCommandsRejectSplitAddressesBeforeTransport(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		for _, name := range []string{"repo", "object", "aspect", "member", "include", "exclude"} {
			t.Run(path+"/"+name, func(t *testing.T) {
				remote := &fakeClient{}
				flags := positionalReferenceFlags(t, path, testReference, map[string]any{name: "obsolete"})
				if strings.HasPrefix(path, "writer ") {
					flags["command-id"] = "must-not-write"
					if path == "writer put" {
						flags["value"] = "null"
					}
				}
				_, err := New(remote, Context{}).Execute(context.Background(), path, flags, client.RequestOptions{})
				if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(remote.calls) != 0 {
					t.Fatalf("split address reached transport: %v %#v", err, remote.calls)
				}
				if !strings.Contains(err.Error(), "object URN") || strings.Contains(err.Error(), "pass --repo") {
					t.Fatalf("recovery instruction recommends a retired address: %v", err)
				}
				entry, _ := command(path)
				for _, flag := range entry.Flags {
					if flag == name {
						t.Fatalf("split address still advertised: %s --%s", path, name)
					}
				}
			})
		}
	}
}

func TestBasicObjectRecoveryDoesNotRecommendRetiredScopeFlags(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		for _, name := range []string{"dataset", "pin", "source"} {
			if path == "read" && name == "dataset" {
				continue // Explicit Dataset scope remains a valid read qualifier.
			}
			remote := &fakeClient{}
			flags := positionalReferenceFlags(t, path, testReference, map[string]any{name: "unsupported"})
			_, err := New(remote, Context{}).Execute(context.Background(), path, flags, client.RequestOptions{})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(remote.calls) != 0 || !strings.Contains(err.Error(), "object URN") || strings.Contains(err.Error(), "pass --repo") || strings.Contains(err.Error(), "pass --dataset or --repo") {
				t.Fatalf("%s --%s recovery points to retired addressing: %v %#v", path, name, err, remote.calls)
			}
		}
	}
}

func TestBasicObjectCommandsRequireReferenceBeforeTransport(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		remote := &fakeClient{}
		flags := map[string]any{}
		if strings.HasPrefix(path, "writer ") {
			flags["command-id"] = "must-not-write"
			if path == "writer put" {
				flags["value"] = "null"
			}
		}
		_, err := New(remote, Context{Dataset: "default"}).Execute(context.Background(), path, flags, client.RequestOptions{})
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), "object URN") || len(remote.calls) != 0 {
			t.Fatalf("%s silently selected an object: %v %#v", path, err, remote.calls)
		}
	}
}

func TestLocalModelAndLeafHelpNeedNoClient(t *testing.T) {
	for _, name := range []string{"object_id", "Aspect", "Member", "Address", "Relation", "JSON Pointer", testReference} {
		if !strings.Contains(ModelHelp(), name) {
			t.Fatalf("local root help omits %q", name)
		}
	}
	for _, entry := range Commands() {
		result, err := New(nil, Context{}).Execute(context.Background(), entry.Path, map[string]any{"help": true}, client.RequestOptions{})
		if err != nil || !strings.Contains(result.(string), "kc "+entry.Path) {
			t.Fatalf("%s help needs transport: %v", entry.Path, err)
		}
	}
	entries := Commands()
	entries[0].Flags[0] = "mutated"
	if Commands()[0].Flags[0] == "mutated" {
		t.Fatal("host mutated module contract")
	}
}

func TestBasicObjectHelpShowsOnlyReferenceAddress(t *testing.T) {
	for _, path := range []string{"read", "writer put", "writer remove"} {
		entry := leafHelp[path]
		if !strings.HasPrefix(entry.Usage, "kc "+path+" <object-urn") {
			t.Fatalf("%s lost complete reference syntax: %s", path, entry.Usage)
		}
		for _, old := range []string{"--reference", "--repo", "--object", "--aspect", "--member", "--include", "--exclude", "兼容"} {
			if strings.Contains(entry.Usage, old) {
				t.Fatalf("%s still documents a second address form %q: %s", path, old, entry.Usage)
			}
		}
		metadata, _ := command(path)
		if !reflect.DeepEqual(metadata.Operands, []string{"reference"}) {
			t.Fatalf("%s does not declare exactly one positional target: %#v", path, metadata.Operands)
		}
		for _, flag := range metadata.Flags {
			if flag == "reference" {
				t.Fatalf("%s still advertises the target flag alias", path)
			}
		}
	}
}

func TestClosedLeafInputsRejectCrossOwnerFlags(t *testing.T) {
	for _, flags := range []map[string]any{positionalReferenceFlags(t, "read", testReference, map[string]any{"query": "x"}), positionalReferenceFlags(t, "read", testReference, map[string]any{"reference-extra": true}), {"repo": "r", "object": "x", "aspect": "a"}, {"repo": "r", "object": "x", "pin": "{}"}} {
		path := "read"
		if flags["aspect"] != nil {
			path = "log"
		}
		if err := New(nil, Context{}).Validate(path, flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("accepted wrong-owner/unknown input: %#v %v", flags, err)
		}
	}
}

func TestReferenceDatasetAcceptsExplicitMatchingBasisWithoutGuessing(t *testing.T) {
	remote := &fakeClient{}
	flags := positionalReferenceFlags(t, "read", testReference, map[string]any{"dataset": "delivery", "commit": "fixed"})
	if _, err := New(remote, Context{}).Execute(context.Background(), "read", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if remote.read.Reference != testReference || remote.read.Dataset != "delivery" || remote.read.Commit != "fixed" {
		t.Fatalf("lost explicit basis: %#v", remote.read)
	}
	for _, bad := range []map[string]any{positionalReferenceFlags(t, "read", testReference, map[string]any{"dataset": "delivery", "ref": "published"}), positionalReferenceFlags(t, "read", testReference, map[string]any{"repo": "kr://other"}), {"dataset": "delivery", "object": "table/orders", "commit": "fixed"}} {
		if err := New(remote, Context{}).Validate("read", bad); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("conflicting basis accepted: %#v %v", bad, err)
		}
	}
}

func positionalReferenceFlags(t *testing.T, path, reference string, flags map[string]any) map[string]any {
	t.Helper()
	if err := BindOperands(path, flags, []string{reference}); err != nil {
		t.Fatal(err)
	}
	return flags
}
