package dataset_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func catalogFromRegistry(t *testing.T, registry catalog.Registry, repositoryIDs ...string) catalog.Service {
	t.Helper()
	store := snapshot.NewRegistry()
	for _, id := range repositoryIDs {
		if err := store.Add(testkit.MakeRepository(t, id)); err != nil {
			t.Fatal(err)
		}
	}
	cat, err := catalog.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

type registryIdentityAlias struct {
	*catalogMem
}

func (registryIdentityAlias) ID() kernel.RepositoryID { return "kr://wrong/authority" }

func TestSnapshotRegistryRequiresExplicitCreationAndRecoversNewHandle(t *testing.T) {
	const id = "kr://remote/catalog"
	authority := newCatalogMem(id)
	const ref = "refs/heads/catalog"
	root, _ := authority.Head(snapshot.DefaultRef)
	if err := authority.CreateRef(ref, root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.OpenSnapshotRegistry(authority, ref); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("missing authority must not be created by open: %v", err)
	}
	registry, err := catalog.CreateSnapshotRegistry(authority, ref)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalogFromRegistry(t, registry, "kr://remote/source")
	if err := cat.RegisterRepository("kr://remote/source"); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.DefineKnowledgeSet("task", 1, []catalog.KnowledgeSetSource{{Repository: "kr://remote/source", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	if err := cat.RetireKnowledgeSet("task"); err != nil {
		t.Fatal(err)
	}
	head, err := registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	before := catalog.NormalizeState(cat.DumpState())

	reopened, err := catalog.OpenSnapshotRegistry(authority, ref)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.Load()
	if err != nil || !reflect.DeepEqual(before, catalog.NormalizeState(state)) {
		t.Fatalf("authority recovery: %#v %v", state, err)
	}
	if got, err := reopened.Head(); err != nil || got != head {
		t.Fatalf("read-only open changed authority basis: %s %v", got, err)
	}
	if _, err := catalog.CreateSnapshotRegistry(authority, ref); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("create overwrote existing ref: %v", err)
	}
	if _, err := catalog.OpenSnapshotRegistry(registryIdentityAlias{authority}, ref); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("wrong identity accepted: %v", err)
	}
	if history := catalogFromRegistry(t, reopened).Log(catalog.LogQuery{}); len(history.Commits) != 4 {
		t.Fatalf("history was not recovered: %#v", history)
	}
}

func TestSnapshotRegistryConcurrentWritersUseAuthorityCAS(t *testing.T) {
	const id = "kr://concurrent/catalog"
	authority := newCatalogMem(id)
	if _, err := catalog.CreateSnapshotRegistry(authority, ""); err != nil {
		t.Fatal(err)
	}
	var cats [2]catalog.Service
	var registries [2]catalog.Registry
	for i := range cats {
		registry, err := catalog.OpenSnapshotRegistry(authority, "")
		if err != nil {
			t.Fatal(err)
		}
		registries[i] = registry
		cats[i] = catalogFromRegistry(t, registry)
	}
	start := make(chan struct{})
	var errors [2]error
	var wg sync.WaitGroup
	for i := range cats {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errors[i] = cats[i].RegisterRepository(kernel.RepositoryID("kr://concurrent/source-" + string(rune('a'+i))))
		}(i)
	}
	close(start)
	wg.Wait()
	winner, loser := 0, 1
	if errors[0] != nil {
		winner, loser = 1, 0
	}
	if errors[winner] != nil || kernel.CodeOf(errors[loser]) != kernel.ErrNonFastForward {
		t.Fatalf("need one accepted CAS and one conflict: %v", errors)
	}
	if len(cats[loser].Repositories()) != 0 {
		t.Fatalf("rejected writer leaked state: %#v", cats[loser].DumpState())
	}
	loserHead, _ := registries[loser].Head()
	winnerHead, _ := registries[winner].Head()
	if loserHead == winnerHead {
		t.Fatal("rejected handle changed its accepted basis")
	}
	retryRegistry, err := catalog.OpenSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	retry := catalogFromRegistry(t, retryRegistry)
	if err := retry.RegisterRepository(kernel.RepositoryID("kr://concurrent/source-" + string(rune('a'+loser)))); err != nil {
		t.Fatal(err)
	}
	if len(retry.Repositories()) != 2 {
		t.Fatalf("retry lost previously accepted change: %#v", retry.DumpState())
	}
}

func TestSnapshotRegistryRejectedWriteLeavesStateAndAuthorityUnchanged(t *testing.T) {
	authority, registry := createCatalogRegistry(t, "kr://retry/catalog")
	cat := catalogFromRegistry(t, registry)
	head, _ := registry.Head()
	authority.applyErr = errors.New("authority rejected candidate")
	if err := cat.RegisterRepository("kr://retry/source"); err == nil {
		t.Fatal("authority rejected candidate but mutation succeeded")
	}
	if cat.HasRepository("kr://retry/source") {
		t.Fatal("failed write leaked registration")
	}
	if got, _ := registry.Head(); got != head {
		t.Fatal("failed write moved handle HEAD")
	}
	if got, err := authority.Head(snapshot.DefaultRef); err != nil || string(got) != head {
		t.Fatalf("failed write moved authority HEAD: %s %v", got, err)
	}
	authority.applyErr = nil
	if err := cat.RegisterRepository("kr://retry/source"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	recovered, err := catalog.OpenSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	if !catalogFromRegistry(t, recovered).HasRepository("kr://retry/source") {
		t.Fatal("retry not present in authority")
	}
}
