package dataset_test

import (
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func makeRepo(t *testing.T, repoID string, objects map[string]any) *testkit.ObjectRepository {
	t.Helper()
	repo := testkit.MakeRepository(t, repoID)
	head := testkit.MustHead(t, repo, "refs/heads/main")
	for objectID, value := range objects {
		var err error
		head, err = repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
			TargetRepository: kernel.RepositoryID(repoID), TargetRef: "refs/heads/main",
			BaseCommit: head, ExpectedTargetCommit: head,
			Operations: testkit.PutEntity(objectID, value, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

type fed struct {
	catalog    catalog.Service
	store      snapshot.Registry
	publicRepo *testkit.ObjectRepository
	registry   catalog.Registry
	authority  *catalogMem
}

func setupFed(t *testing.T) fed {
	t.Helper()
	publicRepo := makeRepo(t, "kr://acme/public/core", map[string]any{"policy/P-103": map[string]any{"statement": "public v1"}})
	groupRepo := makeRepo(t, "kr://acme/groups/payments", map[string]any{
		"policy/P-103":   map[string]any{"statement": "group qualification"},
		"assertion/A-27": map[string]any{"about": "policy/P-103"},
	})
	personalRepo := makeRepo(t, "kr://acme/personals/alice", map[string]any{"note/oncall": map[string]any{"text": "check freeze"}})
	store := snapshot.NewRegistry()
	for _, repo := range []*testkit.ObjectRepository{publicRepo, groupRepo, personalRepo} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	authority, registry := createCatalogRegistry(t, "kr://acme/catalog")
	cat, err := catalog.NewService(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	testkit.RegisterMounted(t, cat, store)
	return fed{catalog: cat, store: store, publicRepo: publicRepo, registry: registry, authority: authority}
}

func TestT11ResolveView(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineDataset("alice-default", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main"},
		{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	r1, err := s.catalog.ResolveDataset("alice-default")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.catalog.ResolveDataset("alice-default")
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Repositories) != 3 || len(r2.Repositories) != 3 {
		t.Fatal(r1, r2)
	}
	if r1.Repositories["kr://acme/public/core"] != r2.Repositories["kr://acme/public/core"] {
		t.Fatal(r1, r2)
	}
}

func TestT11RejectsDuplicateAndUnresolved(t *testing.T) {
	s := setupFed(t)
	_, err := s.catalog.DefineDataset("dup", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	})
	testkit.ExpectCode(t, err, kernel.ErrDatasetInvalid)
	if _, err := s.catalog.DefineDataset("bad-ref", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/missing"},
	}); err == nil {
		t.Fatal("publish must fail when the selector cannot be frozen to a commit")
	}
}

func TestT11FederatedReadDoesNotOverride(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineDataset("v", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	results, err := testkit.FederatedRead(s.catalog, "v", "policy/P-103")
	if err != nil || len(results) != 2 {
		t.Fatal(results, err)
	}
	byRepo := map[kernel.RepositoryID]any{}
	for _, r := range results {
		byRepo[r.Repository] = r.Value
		if r.Commit == "" {
			t.Fatal("missing commit")
		}
	}
	if byRepo["kr://acme/public/core"].(map[string]any)["statement"] != "public v1" {
		t.Fatal(byRepo)
	}
	if byRepo["kr://acme/groups/payments"].(map[string]any)["statement"] != "group qualification" {
		t.Fatal(byRepo)
	}
}

func TestT11PropagatesUnmounted(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineDataset("v", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	absent, err := testkit.FederatedRead(s.catalog, "v", "absent")
	if err != nil || len(absent) != 0 {
		t.Fatal(absent, err)
	}
	s.store.Delete("kr://acme/public/core")
	_, err = testkit.FederatedRead(s.catalog, "v", "absent")
	testkit.ExpectCode(t, err, kernel.ErrDatasetInvalid)
}

func TestT11ReadViewFollowsBranch(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineDataset("v", 1, []catalog.DatasetSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	first, err := testkit.FederatedRead(s.catalog, "v", "policy/P-103")
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	if first[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal(first[0].Value)
	}
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.publicRepo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"statement": "later"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	next, err := testkit.FederatedRead(s.catalog, "v", "policy/P-103")
	if err != nil || next[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal("published dataset must stay frozen until republish", next, err)
	}
	if _, err := s.catalog.DefineDataset("v", 2, []catalog.DatasetSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	next, err = testkit.FederatedRead(s.catalog, "v", "policy/P-103")
	if err != nil || next[0].Value.(map[string]any)["statement"] != "later" {
		t.Fatal(next, err)
	}
	_, err = testkit.FederatedRead(s.catalog, "missing", "policy/P-103")
	testkit.ExpectCode(t, err, kernel.ErrDatasetInvalid)
}

func TestT11NilRegistryRejected(t *testing.T) {
	_, err := catalog.NewService(snapshot.NewRegistry(), nil)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestT11RegistrySurvives(t *testing.T) {
	s := setupFed(t)
	cat := s.catalog
	if _, err := cat.DefineDataset("v", 1, []catalog.DatasetSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	again, err := catalog.NewService(s.store, s.registry)
	if err != nil {
		t.Fatal(err)
	}
	read, err := testkit.FederatedRead(again, "v", "policy/P-103")
	if err != nil || read[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal(read, err)
	}
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.publicRepo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"statement": "later"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	read, err = testkit.FederatedRead(again, "v", "policy/P-103")
	if err != nil || read[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal("reloaded catalog must keep the frozen published commit", read, err)
	}
}

func TestT11GitRegistryHistory(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineDataset("v", 1, []catalog.DatasetSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.catalog.ResolveDataset("v")
	if err != nil {
		t.Fatal(err)
	}
	check := s.catalog.CheckResolved(resolved)
	if check.Outcome != "PASSED" {
		t.Fatal(check)
	}
	again, err := catalog.NewService(s.store, s.registry)
	if err != nil {
		t.Fatal(err)
	}
	history, err := again.Log(catalog.LogQuery{Dataset: "v", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	viewLog := history.Commits
	sawDefine := false
	for _, item := range viewLog {
		if strings.HasPrefix(item.Message, "dataset-define") {
			sawDefine = true
		}
	}
	if !sawDefine {
		t.Fatal(viewLog)
	}
	s.store.Delete("kr://acme/public/core")
	failed := again.CheckResolved(resolved)
	if failed.Outcome != "FAILED" || failed.Issues[0].Code != kernel.ErrUsageInvalid {
		t.Fatal(failed)
	}
}
