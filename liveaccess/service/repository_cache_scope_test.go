package service_test

import (
	"context"
	"fmt"
	"testing"

	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	readcache "kc/object/cache"
	"kc/object/reader"
	"kc/snapshot"
)

type repositoryCacheScopeGuard struct {
	snapshot.Store
	snapshot.TreeStore
	blocked map[string]bool
	reads   map[string]int
}

func (g *repositoryCacheScopeGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	g.reads[path]++
	if g.blocked[path] {
		return nil, fmt.Errorf("cached scoped read touched a forbidden body: %s", path)
	}
	return g.TreeStore.ReadFile(path, at)
}

func (g *repositoryCacheScopeGuard) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, fmt.Errorf("cached scoped read scanned the repository")
}

func TestRepositoryReadHotCachePreservesNestedDatasetScope(t *testing.T) {
	id := objectmodel.ObjectID("widget/cache-scope")
	public := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "public"}
	private := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "private"}
	var guard *repositoryCacheScopeGuard
	_, repo, commit, _ := publishUnitFixture(t, id, []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: public, PathHint: "public/selected.yaml", Value: "published"},
		{Op: objectmodel.OpPut, Address: private, PathHint: "private/secret.yaml", Value: "private"},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "sibling"},
			PathHint: "public/sibling.yaml", Value: "unselected"},
	}, func(store snapshot.Store) snapshot.Store {
		guard = &repositoryCacheScopeGuard{Store: store, TreeStore: store.(snapshot.TreeStore), blocked: map[string]bool{}, reads: map[string]int{}}
		return guard
	})
	cache, err := readcache.New(readcache.Config{MaxEntries: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	if _, err := cache.ReadMany(repo, commit, []objectmodel.ObjectID{id}); err != nil {
		t.Fatal(err)
	}
	for _, address := range []objectmodel.Address{public, private} {
		if _, err := cache.ReadAddress(repo, commit, address); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"public/selected.yaml", "private/secret.yaml", "public/sibling.yaml"} {
		guard.blocked[path] = true
	}
	guard.reads = map[string]int{}
	request := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "consumer"}}
	warmHits := cache.Stats().Hits
	for _, selection := range []liveservice.ReadRequest{{Commit: commit, Object: id, Hydrator: cache}, {Commit: commit, Address: &public, Hydrator: cache}} {
		result, err := liveservice.ReadRepository(context.Background(), repo, selection, nil, request)
		if err != nil || result.Value == nil {
			t.Fatalf("ordinary Repository lost its hot cache: %#v %v", result, err)
		}
	}
	if cache.Stats().Hits != warmHits+2 {
		t.Fatalf("ordinary Repository did not retain both cached read shapes: %#v", cache.Stats())
	}
	guard.blocked["public/selected.yaml"] = false
	commits := map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{
		Repositories: commits, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Kind: "prefix", Prefix: "public"}},
	})
	member, err := scope.Member(repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	nested := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return member, nil }, reader.DatasetPin{
		Repositories: commits, Items: reader.WholeRepositoryItems(commits),
	})
	nestedMember, err := nested.Member(repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	for name, selected := range map[string]objectmodel.Repository{"partial": member, "nested-whole": nestedMember} {
		t.Run(name, func(t *testing.T) {
			guard.reads = map[string]int{}
			hits := cache.Stats().Hits
			for _, selection := range []liveservice.ReadRequest{{Commit: commit, Object: id, Hydrator: cache}, {Commit: commit, Address: &private, Hydrator: cache}} {
				result, err := liveservice.ReadRepository(context.Background(), selected, selection, nil, request)
				if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved || result.Value != nil {
					t.Errorf("hot cache widened the supplied Dataset scope for Address %v: value=%#v %v", selection.Address, result.Value, err)
				}
			}
			result, err := liveservice.ReadRepository(context.Background(), selected, liveservice.ReadRequest{Commit: commit, Address: &public, Hydrator: cache}, nil, request)
			if err != nil || result.Value != "published" || result.Address != public || guard.reads["public/selected.yaml"] != 1 {
				t.Fatalf("published partial Address did not use the exact scoped path: %#v reads=%v %v", result, guard.reads, err)
			}
			if cache.Stats().Hits != hits || guard.reads["private/secret.yaml"] != 0 || guard.reads["public/sibling.yaml"] != 0 {
				t.Fatalf("partial read consumed whole-scope cache or private/sibling bodies: stats=%#v reads=%v", cache.Stats(), guard.reads)
			}
		})
	}
}
