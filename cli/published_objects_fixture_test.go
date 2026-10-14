package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	kcclient "kc/client"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	serv "kc/server"
	"kc/snapshot"
)

type publishedObjectFixture struct {
	server, repository, resourceRepository, otherRepository string
	resourceCommit, otherCommit, otherHead                  string
	resourceStore, otherStore                               snapshot.Store
	offlineOrigin                                           string
	offlineCalls                                            atomic.Int64
}

func publishedObjectMaterials() string {
	return filepath.Join(scenesRoot(), "service-initialized", "source-repositories-configured", "repository-attached", "_materials", "published-objects")
}

func newPublishedObjectFixture(t *testing.T, indexed bool) *publishedObjectFixture {
	t.Helper()
	f := &publishedObjectFixture{repository: "kr://test/published-objects/" + strings.ToLower(t.Name()),
		resourceRepository: "kr://test/object-resources/" + strings.ToLower(t.Name()),
		otherRepository:    "kr://test/other-resources/" + strings.ToLower(t.Name())}
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init"))
	if indexed && strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required for published-object discovery acceptance")
	}
	if !indexed {
		body(t, kc(home, "store-set", "--index", "none"))
	}
	if indexed {
		seedRepo(t, home, f.repository)
	} else {
		seedUnindexedRepo(t, home, f.repository)
	}
	fake := testkit.NewLakeFSFake(t)
	f.resourceStore, f.resourceCommit = publishObjectResources(t, home, fake, f.resourceRepository,
		[]string{"media/tea-tin.png", "src/tax/calculate.js", "test-vectors/tax.json"})
	f.otherStore, f.otherCommit = publishObjectResources(t, home, fake, f.otherRepository, []string{"legal/LICENSE.txt"})
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "agent:object-governor"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	f.server = server.URL
	for _, role := range []struct{ principal, actions string }{
		{"agent:object-publisher", "writer.commit,writer.preview,knowledge.read"},
		{"agent:object-consumer", "knowledge.read,knowledge.search,knowledge.schema.read,resource.access"},
	} {
		body(t, f.govern(t, "grant", "add", "--principal", role.principal, "--repo", f.repository, "--action", role.actions))
	}
	body(t, f.govern(t, "writer", "put", testObjectReference(t, f.otherRepository, "private/policy", ""), "--command-id", "other-private-policy", "--value", `{"message":"private resource owner"}`))
	f.otherHead = f.head(t, f.otherRepository)
	offline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.offlineCalls.Add(1)
		http.Error(w, "table source unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(offline.Close)
	f.offlineOrigin = offline.URL
	return f
}

// These are pre-existing raw Snapshot resources, outside the knowledge Writer.
func publishObjectResources(t *testing.T, home string, fake *testkit.LakeFSFake, repository string, paths []string) (snapshot.Store, string) {
	t.Helper()
	physical := fake.NewRepo()
	store, err := snapshot.OpenLakeFS(kernel.RepositoryID(repository), fake.DSN(physical), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := store.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	before, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	var changes []snapshot.TreeChange
	for _, path := range paths {
		raw, err := os.ReadFile(filepath.Join(publishedObjectMaterials(), "resources", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, ".png") {
			decoded, err := png.Decode(bytes.NewReader(raw))
			if err != nil || decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 {
				t.Fatalf("invalid raw picture fixture: %v", err)
			}
		}
		changes = append(changes, snapshot.TreeChange{Path: path, Content: raw})
	}
	commit, err := fake.CommitFiles(physical, before, changes)
	if err != nil {
		t.Fatal(err)
	}
	seedUnindexedRepo(t, home, repository, "--driver", "lakefs", "--dsn", fake.DSN(physical))
	return store, string(commit)
}

func (f *publishedObjectFixture) govern(t *testing.T, args ...string) kcRunResult {
	t.Helper()
	return kcRemote(t, f.server, "agent:object-governor", args...)
}
func (f *publishedObjectFixture) provide(t *testing.T, args ...string) kcRunResult {
	t.Helper()
	return kcRemote(t, f.server, "agent:object-publisher", args...)
}
func (f *publishedObjectFixture) consume(t *testing.T, args ...string) kcRunResult {
	t.Helper()
	return kcRemote(t, f.server, "agent:object-consumer", args...)
}
func (f *publishedObjectFixture) head(t *testing.T, repository string) string {
	t.Helper()
	return asMap(t, body(t, f.govern(t, "writer", "head", "--repo", repository)))["commit"].(string)
}

func (f *publishedObjectFixture) publish(t *testing.T, kind string) (string, map[string]any) {
	t.Helper()
	draft := t.TempDir()
	replacements := strings.NewReplacer("__RESOURCE_REPO__", f.resourceRepository, "__RESOURCE_COMMIT__", f.resourceCommit,
		"__OTHER_REPO__", f.otherRepository, "__OTHER_COMMIT__", f.otherCommit, "__OFFLINE_ORIGIN__", f.offlineOrigin)
	var definition map[string]any
	for _, source := range []string{"common", kind} {
		root := filepath.Join(publishedObjectMaterials(), source)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := replacements.Replace(string(raw))
			if strings.HasSuffix(entry.Name(), ".description.aspect.yaml") {
				parts := strings.SplitN(content, "\n---\n", 2)
				if len(parts) != 2 {
					t.Fatalf("missing description body in %s", path)
				}
				if err := json.Unmarshal([]byte(parts[1]), &definition); err != nil {
					return err
				}
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			target := filepath.Join(draft, relative)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.WriteFile(target, []byte(content), 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if definition == nil {
		t.Fatal("description material was not loaded")
	}
	published := asMap(t, body(t, f.provide(t, "writer", "commit", "--command-id", "publish-"+kind,
		"--repo", f.repository, "--dir", draft)))
	commit := publishedCommit(t, published)
	if f.head(t, f.repository) != commit {
		t.Fatal("accepted object publication was not visible at the returned commit")
	}
	return commit, definition
}

func (f *publishedObjectFixture) verifyDiscovery(t *testing.T, kind, object, keyword, commit string, definition map[string]any) {
	t.Helper()
	schema := asMap(t, body(t, f.consume(t, "read", testObjectReference(t, f.repository, "schema/published-object.definition/v1", ""), "--commit", commit)))
	schemaValue := asMap(t, schema["value"])
	fields, ok := schemaValue["fields"].(map[string]any)
	if !ok {
		t.Fatalf("published Schema read has no field contract: %#v", schema)
	}
	if schema["commit"] != commit || asMap(t, schema["value"])["entity"] != "PublishedObject" ||
		!reflect.DeepEqual(asMap(t, fields["kind"])["access"], []any{"filter"}) {
		t.Fatalf("instance and its declared discovery fields do not share the publication basis: %#v", schema)
	}
	body(t, f.govern(t, "operations", "projection", "sync", "--repo", f.repository))
	consumer := productClient(t, f.server, "agent:object-consumer").KnowledgeService()
	for _, query := range [][]string{{"--query", keyword}, {"--eq", "kind=" + kind}} {
		args := append([]string{"search", "--repo", f.repository, "--commit", commit}, query...)
		search := asMap(t, body(t, f.consume(t, args...)))
		if len(search["hits"].([]any)) != 1 || searchHitObjectID(t, search) != object || search["completeness"] != "complete" {
			t.Fatalf("declared discovery did not identify the published object: %#v", search)
		}
		hit := asMap(t, search["hits"].([]any)[0])
		if hit["repository"] != f.repository || hit["commit"] != commit || hit["objectId"] != object {
			t.Fatalf("discovery lost object identity or fixed version: %#v", hit)
		}
		request := kcclient.KnowledgeSearchRequest{KnowledgeScope: kcclient.KnowledgeScope{Repository: f.repository, Commit: commit}}
		if query[0] == "--query" {
			request.Query = keyword
		} else {
			request.Equal = []string{"kind=" + kind}
		}
		var result retrieval.SearchResult
		if err := consumer.Search(context.Background(), request, kcclient.RequestOptions{}, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Hits) != 1 {
			t.Fatalf("typed discovery did not identify exactly one object: %#v", result)
		}
		fixed := result.Hits[0]
		if string(fixed.Knowledge.Repository) != f.repository || string(fixed.Knowledge.Commit) != commit ||
			string(fixed.Version.ObjectID) != object || string(fixed.Version.DeclarationCommit) != commit {
			t.Fatalf("typed discovery lost object identity or fixed version: %#v", fixed)
		}
		read := asMap(t, body(t, f.consume(t, "read", testObjectReference(t, f.repository, object, ""), "--commit", commit)))
		if !reflect.DeepEqual(read["value"], fixed.Knowledge.Value) || !reflect.DeepEqual(asMap(t, read["value"])["definition"], definition) {
			t.Fatalf("hit and authoritative same-version read diverged: hit=%#v read=%#v", fixed, read)
		}
	}
	private := asMap(t, body(t, f.consume(t, "search", "--repo", f.repository, "--commit", commit, "--query", "hiddenassetnotes")))
	if len(private["hits"].([]any)) != 0 {
		t.Fatalf("a field without access was searched: %#v", private)
	}
	expectCode(t, f.consume(t, "search", "--repo", f.repository, "--commit", commit, "--eq", "internalNote=hiddenassetnotes"), "CAPABILITY_UNSATISFIED")
	f.verifyUninstalledInspection(t, object, commit)
}

func (f *publishedObjectFixture) verifyUninstalledInspection(t *testing.T, object, commit string) {
	t.Helper()
	expectCode(t, f.consume(t, "invoke", "--repo", f.repository, "--commit", commit, "--object", object,
		"--implementation", "inspect", "--operation", "inspect", "--input", "{}"), "CAPABILITY_UNSATISFIED")
}

func (f *publishedObjectFixture) verifyResourceFile(t *testing.T, store snapshot.Store, commit, path string) {
	t.Helper()
	reader, ok := store.(snapshot.TreeReader)
	if !ok {
		t.Fatal("raw resource fixture has no file reader")
	}
	raw, err := reader.ReadFile(path, kernel.CommitID(commit))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join(publishedObjectMaterials(), "resources", filepath.FromSlash(path)))
	if err != nil || !bytes.Equal(raw, expected) {
		t.Fatalf("object processing changed raw resource bytes: %s err=%v", path, err)
	}
}
