package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"kc/discovery/client"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

type pluginFileHTTPTransport struct{ client *http.Client }

func (tr pluginFileHTTPTransport) Do(ctx context.Context, _ string, request *http.Request) (*http.Response, error) {
	request = request.WithContext(ctx)
	request.Header.Set("X-Kc-As", "reader")
	return tr.client.Do(request)
}

// JSON transport would replace invalid bytes and change what is judged, so the
// CLI refuses such a file before any request.
func TestPluginTrialRefusesNonUTF8DeclarationFile(t *testing.T) {
	valid := "---\nobject_id: index/fixture\naspect_name: index\nmember_key: body\nschema_ref: " + string(objectmodel.IndexDeclarationSchemaV1) +
		"\n---\ncatalog: " + testkit.IndexDeclarationCatalog + "\ninputs:\n  - from: schema/article#/fields/body\nbuild:\n  kind: text\nuses: [text]\n"
	invalid := strings.Replace(valid, "\n---\n", "\n---\n# invalid comment: \xff\n", 1)
	// The same document changes from invalid to valid if JSON silently replaces
	// its bad byte. Keep that counterexample attached to the real CLI request.
	encoded, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	var replaced string
	if err := json.Unmarshal(encoded, &replaced); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		content string
		valid   bool
	}{{invalid, false}, {replaced, true}} {
		unit := repofile.Parse(tc.content)
		if unit == nil {
			t.Fatal("fixture lost publication frontmatter")
		}
		_, err := objectmodel.ParseIndexDeclaration(unit.Address, unit.Value)
		if (err == nil) != tc.valid {
			t.Fatalf("fixture does not demonstrate changed declaration legality: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "decl.yaml")
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	sent := make(chan client.DeclarationTrialRequest, 1)
	network := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var request client.DeclarationTrialRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sent <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"members":[]}`))
	}))
	t.Cleanup(network.Close)
	remote, err := client.New(client.Config{BaseURL: network.URL, Transport: pluginFileHTTPTransport{network.Client()}})
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]any{"name": "declarative-index", "task": "trial", "repo": "kr://repo/public", "file": path}
	_, err = New(remote, Context{}).Execute(context.Background(), "plugin", flags, client.RequestOptions{})
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("non-UTF-8 declaration file was sent: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("invalid declaration reached the HTTP transport")
	}
	valid = strings.Replace(valid, "\n---\n", "\n---\n# Unicode 正文\n", 1)
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(remote, Context{}).Execute(context.Background(), "plugin", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	request := <-sent
	if requests.Load() != 1 || len(request.Documents) != 1 || request.Documents[0].Content != valid || request.Documents[0].Path != "decl.yaml" {
		t.Fatalf("valid Unicode file was not transmitted unchanged: %#v", request)
	}
}
