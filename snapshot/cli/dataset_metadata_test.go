package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/kernel"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

func TestDatasetMetadataUsesFlagOrPortableRecipe(t *testing.T) {
	recipe := filepath.Join(t.TempDir(), "recipe.yaml")
	if err := os.WriteFile(recipe, []byte("name: notes\nmetadata:\n  description: recipe\n  custom/team: tea\nmounts:\n- repository: repo-A\n  selector: refs/heads/main\n  path: reference\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		flags map[string]any
		want  map[string]string
	}{
		{"recipe", map[string]any{"file": recipe}, map[string]string{"description": "recipe", "custom/team": "tea"}},
		{"override", map[string]any{"file": recipe, "metadata": `{"description":"override","empty":""}`}, map[string]string{"description": "override", "empty": ""}},
		{"clear", map[string]any{"file": recipe, "metadata": `{}`}, nil},
		{"sources", map[string]any{"source": "repo-A@reference", "metadata": `{"arbitrary":"value"}`}, map[string]string{"arbitrary": "value"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request client.DatasetRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if r.URL.Path != "/snapshot/v1/datasets" || request.Dataset != "notes" || request.Revision != 1 || !reflect.DeepEqual(request.Metadata, test.want) {
					t.Errorf("metadata lost on remote define: %s %#v", r.URL.Path, request)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"setId": "notes", "revision": 1})
			}))
			defer server.Close()
			remote, err := client.New(client.Config{BaseURL: server.URL, Transport: &testTransport{client: server.Client()}})
			if err != nil {
				t.Fatal(err)
			}
			test.flags["dataset"], test.flags["revision"] = "notes", "1"
			if _, err := snapshotcli.New(remote, snapshotcli.Context{}).Execute(context.Background(), "dataset define", test.flags, client.RequestOptions{}); err != nil || calls != 1 {
				t.Fatalf("publication: %v, calls=%d", err, calls)
			}
		})
	}
}

func TestDatasetMetadataFlagRejectsInvalidValuesBeforeRemoteRequest(t *testing.T) {
	for _, value := range []string{`null`, `{"x":null}`, `{"x":1}`, `{"x":[]}`, `{"x":"a","x":"b"}`, `{}`, ""} {
		err := snapshotcli.New(nil, snapshotcli.Context{}).Validate("dataset define", map[string]any{"dataset": "notes", "revision": "1", "source": "repo-A@reference", "metadata": value})
		if value == `{}` {
			if err != nil {
				t.Fatalf("empty object must clear metadata: %v", err)
			}
		} else if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("invalid metadata %s accepted: %v", value, err)
		}
	}
}
