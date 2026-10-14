package cli

import (
	"context"
	"reflect"
	"testing"

	"kc/discovery/client"
)

// Existing query tasks consume the last scalar value while retaining every
// filter repetition. Moving their adaptation must preserve the argv vocabulary.
func TestDiscoveryCLIRepeatedScalarFlagsKeepLastValue(t *testing.T) {
	cases := []struct {
		path    string
		flags   map[string]any
		request any
	}{
		{
			path: "search",
			flags: map[string]any{
				"repo": []string{"old-repo", "r"}, "commit": []string{"old-commit", "c"}, "ref": []string{"old-ref", "main"},
				"query": []string{"old-query", "current"}, "match-mode": []string{"old-mode", "all"},
				"eq": []string{"amount=1", "amount=2"}, "sort": []string{"name:asc", "amount:desc"},
				"limit": []string{"invalid-old-limit", "7"}, "continuation": []string{"old-cursor", "current-cursor"},
			},
			request: client.SearchRequest{
				KnowledgeScope: client.KnowledgeScope{Repository: "r", Commit: "c", Ref: "main"},
				Query:          "current", MatchMode: "all", Equal: []string{"amount=1", "amount=2"}, Sort: []string{"name:asc", "amount:desc"},
				Limit: 7, Continuation: "current-cursor",
			},
		},
		{
			path: "relations",
			flags: map[string]any{
				"repo": []string{"old-repo", "r"}, "object": []string{"old-object", "Table:orders"},
				"direction": []string{"old-direction", "DIRECTED"}, "limit": []string{"invalid-old-limit", "7"},
				"continuation": []string{"old-cursor", "current-cursor"},
			},
			request: client.RelationsRequest{
				KnowledgeScope: client.KnowledgeScope{Repository: "r"}, Endpoint: "Table:orders", Direction: "DIRECTED", Limit: 7, Continuation: "current-cursor",
			},
		},
		{
			path: "traverse",
			flags: map[string]any{
				"dataset": []string{"old-set", "d"}, "object": []string{"old-object", "Table:orders"},
				"max-hops": []string{"invalid-old-max", "2"}, "min-hops": []string{"invalid-old-min", "1"},
				"limit": []string{"invalid-old-limit", "7"}, "continuation": []string{"old-cursor", "current-cursor"},
			},
			request: client.TraverseRequest{
				KnowledgeScope: client.KnowledgeScope{Dataset: "d"}, Endpoint: "Table:orders", MinHops: 1, MaxHops: 2, Limit: 7, Continuation: "current-cursor",
			},
		},
		{
			path:    "operations access-spec describe",
			flags:   map[string]any{"dataset": []string{"old-set", "d"}},
			request: client.AccessSpecDescribeRequest{Dataset: "d"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			remote := &recordingClient{}
			if _, err := New(remote, Context{}).Execute(context.Background(), tc.path, tc.flags, client.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if remote.calls != 1 || !reflect.DeepEqual(remote.request, tc.request) {
				t.Fatalf("existing scalar/filter input changed: calls=%d request=%#v want=%#v", remote.calls, remote.request, tc.request)
			}
		})
	}
}
