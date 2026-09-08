package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/controlplane"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
	readcache "kc/retrieval/cache"
	"kc/snapshot"
)

func TestHTTPCachedReadRechecksRevocationAndKeepsOldBasis(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/cache-permission")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: testkit.PutEntity("policy/a", map[string]any{"body": "first version"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	cache, err := readcache.New(readcache.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	rd := reader.NewReader(registry)
	rd.SetHydrator(cache)
	dir := t.TempDir()
	ws := &Home{Dir: dir, Store: registry, Reader: rd, Hydrator: cache, ReadCache: cache,
		File: apphome.HomeFile{Catalogs: []apphome.HomeCatalog{{ID: "kr://acme/catalog"}}}, Controls: map[string]controlplane.ControlState{}}
	facade := &httpFacade{home: dir, readHome: ws, options: HTTPServerOptions{AuthMode: "local"}}
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	grant := func(enabled bool) {
		file := AllowFile{Version: allowVersion}
		if enabled {
			file.Rules = []AllowRule{{ID: "read", Principal: "reader", Repo: string(repo.ID()), Actions: []string{"knowledge.read"}}}
		}
		if err := WriteAllow(dir, file); err != nil {
			t.Fatal(err)
		}
	}
	read := func(basis kernel.CommitID) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{"repository": repo.ID(), "commit": basis, "object": "policy/a"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/knowledge/v1/objects:read", strings.NewReader(string(body)))
		req.Header.Set("X-Kc-As", "reader")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}
	grant(true)
	for i := 0; i < 2; i++ {
		result := read(commit)
		if result.Code != 200 || !strings.Contains(result.Body.String(), "first version") {
			t.Fatalf("read failed: %d %s", result.Code, result.Body.String())
		}
	}
	if cache.Stats().Hits == 0 {
		t.Fatal("HTTP did not use warmed cache")
	}
	grant(false)
	denied := read(commit)
	if denied.Code != http.StatusForbidden || strings.Contains(denied.Body.String(), "first version") {
		t.Fatalf("revocation bypassed: %d %s", denied.Code, denied.Body.String())
	}
	grant(true)
	next, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: commit, ExpectedTargetCommit: commit,
		Operations: testkit.PutEntity("policy/a", map[string]any{"body": "second version"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	for basis, expected := range map[kernel.CommitID]string{commit: "first version", next: "second version"} {
		result := read(basis)
		if result.Code != 200 || !strings.Contains(result.Body.String(), expected) {
			t.Fatalf("basis %s: %d %s", basis, result.Code, result.Body.String())
		}
	}
	// SEARCH may discover this identity without body read permission. Its
	// delivery stage must strip only this response, never the shared cache.
	grant(false)
	value, err := rd.Read(knowledge.KnowledgeRef{Repository: repo.ID(), Object: "policy/a"}, commit, nil)
	if err != nil {
		t.Fatal(err)
	}
	hit, err := deliverSearchHit(dir, map[string]FlagValue{"as": "reader"}, retrieval.KnowledgeHit{Knowledge: value, Version: retrieval.VersionOf(value)})
	if err != nil || hit.Knowledge.Value != nil {
		t.Fatalf("search exposed denied cached body: %+v %v", hit, err)
	}
	grant(true)
	if result := read(commit); result.Code != 200 || !strings.Contains(result.Body.String(), "first version") {
		t.Fatalf("delivery stripped the shared cache: %d %s", result.Code, result.Body.String())
	}
}
