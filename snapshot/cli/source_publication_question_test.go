package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot/access"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

func TestModuleSourcePublicationQuestionUsesPublicationScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rule    access.Rule
		omit    string
		allowed bool
	}{
		{"dataset-only", access.Rule{Dataset: "files"}, "", false},
		{"object-qualified", access.Rule{Repo: "private", Dataset: "files", Ref: "refs/heads/main", Object: "*"}, "", false},
		{"explicit-intersection", access.Rule{Repo: "private", Dataset: "files", Ref: "refs/heads/main"}, "", true},
		{"missing-repository", access.Rule{}, "repo", false},
		{"missing-destination", access.Rule{}, "dataset", false},
		{"missing-ref", access.Rule{}, "ref", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.rule.ID, tc.rule.Principal, tc.rule.Actions = "source-rule", "publisher", []string{access.RepositoryDatasetPublish}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls++
				query := request.URL.Query()
				if request.Method != http.MethodGet || request.URL.Path != "/admin/v1/grants" || query.Get("principal") != "publisher" || query.Get("action") != access.RepositoryDatasetPublish || query.Get("repository") != "" || query.Get("dataset") != "" || query.Get("ref") != "" {
					t.Errorf("source question narrowed or changed the existing global grant-read request: %s", request.URL.RequestURI())
				}
				_ = json.NewEncoder(w).Encode(access.GrantList{Rules: []access.Rule{tc.rule}})
			}))
			defer server.Close()
			sdk, err := client.New(client.Config{BaseURL: server.URL, Transport: &testTransport{client: server.Client()}})
			if err != nil {
				t.Fatal(err)
			}
			flags := map[string]any{"action": access.RepositoryDatasetPublish, "repo": "private", "dataset": "files", "ref": "refs/heads/main"}
			delete(flags, tc.omit)
			result, err := snapshotcli.New(sdk, snapshotcli.Context{Principal: "publisher"}).Execute(context.Background(), "grant list", flags, client.RequestOptions{})
			if tc.allowed {
				if err != nil || !reflect.DeepEqual(result, map[string]any{"allow": true, "ruleId": "source-rule"}) {
					t.Fatalf("explicit source question lost its matching rule: %+v, %v", result, err)
				}
			} else if kernel.CodeOf(err) != kernel.ErrForbidden || result != nil {
				t.Fatalf("invalid source question produced a publication affordance: %+v, %v", result, err)
			}
			if calls != 1 {
				t.Fatalf("question performed extra capability acquisition: %d calls", calls)
			}
		})
	}
}
