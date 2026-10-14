package reader_test

import (
	"reflect"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	readcache "kc/object/cache"
	"kc/object/reader"
	"kc/snapshot"
)

func TestRootReferenceHydrationKeepsFixedContentAndDatasetScope(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	cache, err := readcache.New(readcache.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	rd.SetHydrator(cache)
	ref := objectmodel.Reference{Repository: guard.ID(), Object: "table/orders"}
	first, err := rd.ReadReference(ref, at)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rd.ReadReference(ref, at)
	if err != nil || !reflect.DeepEqual(first, second) || first.Commit != at || first.Repository != ref.Repository || first.Reference == "" || first.Digest != kernel.CanonicalDigest(first.Value) {
		t.Fatalf("root reference changed fixed content or maintenance digest: %#v %#v %v", first, second, err)
	}
	if cache.Stats().Hits == 0 {
		t.Fatal("root Reference did not reuse the injected exact-basis cache")
	}
	lookup := rd.Lookup(func(kernel.RepositoryID) (snapshot.Store, error) { return guard, nil })
	whole := reader.Open(lookup, reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at},
		Items:        reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at}),
	})
	whole.SetHydrator(cache)
	values, err := whole.ReadReference(ref)
	if err != nil || len(values) != 1 || !reflect.DeepEqual(values[0].Value, first.Value) || values[0].Digest != first.Digest {
		t.Fatalf("whole Dataset lost the same reference content: %#v %v", values, err)
	}
	partial := reader.Open(lookup, reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at},
		Items:        []reader.DatasetItem{{Repository: guard.ID(), Commit: at, Kind: "file", File: "public/id.yaml"}},
	})
	partial.SetHydrator(cache)
	before := cache.Stats()
	if _, err := partial.ReadReference(ref); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("warmed whole object escaped a partial Dataset: %v", err)
	}
	if got := cache.Stats(); got.Hits != before.Hits || got.Misses != before.Misses {
		t.Fatalf("partial Dataset consulted the whole-object cache: before=%#v after=%#v", before, got)
	}
	guard.blocked["private/description.yaml"], guard.blocked["private/status.yaml"] = true, true
	ref.Locator = "/columns/order_id/constraints/nullable"
	values, err = partial.ReadReference(ref)
	if err != nil || len(values) != 1 || values[0].Value != false {
		t.Fatalf("selected reference did not keep the scoped unit plan: %#v %v", values, err)
	}
	if got := cache.Stats(); got.Hits != before.Hits || got.Misses != before.Misses {
		t.Fatalf("selected reference fetched a whole object before trimming: before=%#v after=%#v", before, got)
	}
}
