package cli_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/kernel"
	"kc/object/reader"
	serv "kc/server"
)

func TestSearchReadBasisRetainsAllCandidatesAndOutlivesIndexConfiguration(t *testing.T) {
	if os.Getenv("KC_TEST_OPENSEARCH_URL") == "" {
		t.Fatal("search basis integration requires the existing real OpenSearch endpoint")
	}
	home, url, repo, stop := objectReferenceServerLifecycle(t)
	const other = "kr://zz-basis/empty"
	seedRepo(t, home, other)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	drafts := writeProviderDrafts(t)
	// This journey composes Aspects from two sources, so its initial unit is
	// an Aspect too; a whole Entity cannot later be combined with an Aspect.
	for name, replacement := range map[string][2]string{
		"schema.json":  {`"pattern":"record"`, `"aspect":"definition","pattern":"record"`},
		"runbook.json": {"kind: Entity", "kind: Aspect\naspect_name: definition"},
	} {
		path := filepath.Join(drafts, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), replacement[0]) {
			t.Fatalf("fixture %s no longer contains its declared starting model", name)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(raw), replacement[0], replacement[1], 1)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	body(t, run("writer", "commit", "--repo", repo, "--command-id", "search-basis-source", "--dir", drafts))
	body(t, run("dataset", "define", "--dataset", "search-basis", "--revision", "1", "--source", repo, "--source", other))
	body(t, run("grant", "add", "--principal", "consumer", "--dataset", "search-basis", "--action", "file.read"))
	search := func() kcRunResult {
		return kcRemote(t, url, "consumer", "search", "--dataset", "search-basis", "--query", "冻结窗口")
	}
	first := waitRemoteSearchHits(t, search, 1)
	hit := asMap(t, first["hits"].([]any)[0])
	basis, ok := hit["basis"].(string)
	if !ok || basis == "" {
		t.Fatalf("search result lost its reusable object basis: %#v", hit)
	}
	store, err := reader.NewReadBasisStore(filepath.Join(home, "read-bases.db"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Resolve(context.Background(), basis)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Candidates) != 2 || stored.Dataset != "search-basis" || stored.Revision != 1 {
		t.Fatalf("search inferred scope from actual contributors: %#v", stored)
	}
	object := hit["objectId"].(string)
	root := testObjectReference(t, repo, object, "")
	old := asMap(t, body(t, kcRemote(t, url, "consumer", "read", root, "--basis", basis)))
	if asMap(t, old["state"])["digest"] != asMap(t, hit["state"])["digest"] {
		t.Fatalf("search and read disagree about local state: hit=%#v read=%#v", hit, old)
	}
	body(t, run("writer", "put", testObjectReference(t, other, object, "/extra"), "--repo", other, "--command-id", "search-basis-new-contributor", "--value", `{"note":"new contributor"}`))
	body(t, run("dataset", "define", "--dataset", "search-basis", "--revision", "2", "--source", repo, "--source", other))
	second := waitRemoteSearchHits(t, search, 1)
	newHit := asMap(t, second["hits"].([]any)[0])
	if asMap(t, newHit["state"])["scope"] != asMap(t, hit["state"])["scope"] || asMap(t, newHit["state"])["digest"] == asMap(t, hit["state"])["digest"] {
		t.Fatalf("new contribution changed scope or preserved state: old=%#v new=%#v", hit, newHit)
	}
	stop()
	body(t, kc(home, "store-set", "--index", "none"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	reopened := httptest.NewServer(handler)
	t.Cleanup(reopened.Close)
	replay := asMap(t, body(t, kcRemote(t, reopened.URL, "consumer", "read", root, "--basis", basis)))
	if _, expanded := asMap(t, replay["value"])["extra"]; expanded || asMap(t, replay["state"])["digest"] != asMap(t, hit["state"])["digest"] {
		t.Fatalf("old hit followed a newer contributor or required its index: %#v", replay)
	}
	expectCode(t, kcRemote(t, reopened.URL, "consumer", "search", "--dataset", "search-basis", "--query", "冻结窗口"), string(kernel.ErrCapabilityUnsatisfied))
}
