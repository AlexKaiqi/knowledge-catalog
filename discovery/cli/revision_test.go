package cli

import (
	"context"
	"encoding/json"
	"kc/discovery/client"
	"kc/kernel"
	"testing"
)

func TestDatasetRevisionReachesEveryConsumerRequestAndReplacesHostPin(t *testing.T) {
	remote := &recordingClient{}
	e := New(remote, Context{Dataset: "fixed", Pin: json.RawMessage(`{"setId":"fixed"}`)})
	for path, flags := range map[string]map[string]any{
		"search":                          {"dataset": "set", "revision": "2", "query": "q"},
		"relations":                       {"dataset": "set", "revision": "2", "object": "a/b"},
		"traverse":                        {"dataset": "set", "revision": "2", "object": "a/b", "max-hops": "1"},
		"operations access-spec describe": {"dataset": "set", "revision": "2"},
	} {
		if _, err := e.Execute(context.Background(), path, flags, client.RequestOptions{}); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var dataset string
		var revision int
		var pin json.RawMessage
		switch r := remote.request.(type) {
		case client.SearchRequest:
			dataset, revision, pin = r.Dataset, r.Revision, r.Pin
		case client.RelationsRequest:
			dataset, revision, pin = r.Dataset, r.Revision, r.Pin
		case client.TraverseRequest:
			dataset, revision, pin = r.Dataset, r.Revision, r.Pin
		case client.AccessSpecDescribeRequest:
			dataset, revision, pin = r.Dataset, r.Revision, r.Pin
		}
		if dataset != "set" || revision != 2 || len(pin) != 0 {
			t.Fatalf("%s lost the fixed revision or kept the host pin: %#v", path, remote.request)
		}
	}
}

func TestDatasetRevisionRequiresNamedDatasetAndPositiveValue(t *testing.T) {
	e := New(&recordingClient{}, Context{})
	for _, flags := range []map[string]any{
		{"repo": "kr://r", "revision": "1", "query": "q"},
		{"revision": "1", "query": "q"},
		{"dataset": "set", "revision": "0", "query": "q"},
		{"dataset": "set", "revision": "-1", "query": "q"},
		{"dataset": "set", "revision": "two", "query": "q"},
	} {
		if err := e.Validate("search", flags); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid revision accepted: %#v err=%v", flags, err)
		}
	}
}
