package catalog_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func bareCatalogRemote(t *testing.T) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "authority.git")
	if output, err := exec.Command("git", "init", "--bare", "--quiet", remote).CombinedOutput(); err != nil {
		t.Fatalf("bare authority: %s %v", output, err)
	}
	return remote
}

func catalogFromRegistry(t *testing.T, registry *catalog.Registry, repositoryIDs ...string) *catalog.Catalog {
	t.Helper()
	store := snapshot.NewRegistry()
	for _, id := range repositoryIDs {
		if err := store.Add(testkit.MakeRepository(t, id)); err != nil {
			t.Fatal(err)
		}
	}
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestRemoteRegistryRequiresExplicitCreationAndRecoversWithoutCache(t *testing.T) {
	remote := bareCatalogRemote(t)
	const id = "kr://remote/catalog"
	const ref = "refs/heads/catalog"
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := catalog.OpenRemoteRegistry(cache, id, remote, ref); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("missing authority must not be created by open: %v", err)
	}
	registry, err := catalog.CreateRemoteRegistry(cache, id, remote, ref)
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
	if err := cat.Archive(); err != nil {
		t.Fatal(err)
	}
	head, err := registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	before := catalog.NormalizeCatalogState(cat.DumpState())
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	reopened, err := catalog.OpenRemoteRegistry(cache, id, remote, ref)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.Load()
	if err != nil || !reflect.DeepEqual(before, catalog.NormalizeCatalogState(state)) {
		t.Fatalf("cache-free recovery: %#v %v", state, err)
	}
	if got, err := reopened.Head(); err != nil || got != head {
		t.Fatalf("read-only open changed authority basis: %s %v", got, err)
	}
	if _, err := catalog.CreateRemoteRegistry(t.TempDir(), id, remote, ref); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("create overwrote existing ref: %v", err)
	}
	if _, err := catalog.OpenRemoteRegistry(t.TempDir(), "kr://wrong/catalog", remote, ref); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("wrong identity accepted: %v", err)
	}
	if history := catalogFromRegistry(t, reopened).Log(catalog.CatalogLogQuery{}); len(history.Commits) != 5 {
		t.Fatalf("history was not recovered: %#v", history)
	}
}

func TestRemoteRegistryConcurrentWritersUseAuthorityCAS(t *testing.T) {
	remote := bareCatalogRemote(t)
	const id = "kr://concurrent/catalog"
	if _, err := catalog.CreateRemoteRegistry(t.TempDir(), id, remote, ""); err != nil {
		t.Fatal(err)
	}
	var cats [2]*catalog.Catalog
	var registries [2]*catalog.Registry
	for i := range cats {
		registry, err := catalog.OpenRemoteRegistry(t.TempDir(), id, remote, "")
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
	retryRegistry, err := catalog.OpenRemoteRegistry(t.TempDir(), id, remote, "")
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

func TestRemoteRegistryRejectedPushLeavesStateAndCacheHeadUnchanged(t *testing.T) {
	remote := bareCatalogRemote(t)
	registry, err := catalog.CreateRemoteRegistry(t.TempDir(), "kr://retry/catalog", remote, "")
	if err != nil {
		t.Fatal(err)
	}
	cat := catalogFromRegistry(t, registry)
	head, _ := registry.Head()
	hook := filepath.Join(remote, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "--git-dir", remote, "config", "core.hooksPath", filepath.Join(remote, "hooks")).CombinedOutput(); err != nil {
		t.Fatalf("configure rejecting authority hook: %s %v", output, err)
	}
	if err := cat.RegisterRepository("kr://retry/source"); err == nil {
		t.Fatal("authority rejected push but mutation succeeded")
	}
	if cat.HasRepository("kr://retry/source") {
		t.Fatal("failed push leaked registration")
	}
	if got, _ := registry.Head(); got != head {
		t.Fatal("failed push moved handle HEAD")
	}
	if output, err := exec.Command("git", "-C", registry.RootDir(), "rev-parse", "HEAD").Output(); err != nil || strings.TrimSpace(string(output)) != head {
		t.Fatalf("failed push moved cache HEAD: %s %v", output, err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("kr://retry/source"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	recovered, err := catalog.OpenRemoteRegistry(t.TempDir(), registry.CatalogID(), remote, "")
	if err != nil {
		t.Fatal(err)
	}
	if !catalogFromRegistry(t, recovered).HasRepository("kr://retry/source") {
		t.Fatal("retry not present in authority")
	}
}

func TestRemoteRegistryRejectsCacheAsAuthority(t *testing.T) {
	root := t.TempDir()
	for _, pair := range [][2]string{{root, root}, {root, filepath.Join(root, "authority")}, {filepath.Join(root, "cache"), root}} {
		if _, err := catalog.OpenRemoteRegistry(pair[0], "kr://cache/catalog", pair[1], ""); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
			t.Fatalf("nested authority/cache accepted: %v", err)
		}
	}
}
