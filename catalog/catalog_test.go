package catalog_test

import (
	"strings"
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/repository"
)

func makeRepo(t *testing.T, repoID string, objects map[string]any) *local.FileGitRepository {
	t.Helper()
	repo := testkit.MakeRepository(t, repoID)
	head := testkit.MustHead(t, repo, "refs/heads/main")
	for objectID, value := range objects {
		var err error
		head, err = repo.ApplyCommit(repository.CommitChangeSet{
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
	catalog    *catalog.Catalog
	store      *repository.Store
	publicRepo *local.FileGitRepository
	registry   *catalog.Registry
}

func setupFed(t *testing.T) fed {
	t.Helper()
	publicRepo := makeRepo(t, "kr://acme/public/core", map[string]any{"policy/P-103": map[string]any{"statement": "public v1"}})
	groupRepo := makeRepo(t, "kr://acme/groups/payments", map[string]any{
		"policy/P-103":   map[string]any{"statement": "group qualification"},
		"assertion/A-27": map[string]any{"about": "policy/P-103"},
	})
	personalRepo := makeRepo(t, "kr://acme/personals/alice", map[string]any{"note/oncall": map[string]any{"text": "check freeze"}})
	store := repository.NewStore()
	for _, repo := range []*local.FileGitRepository{publicRepo, groupRepo, personalRepo} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	registry := testkit.MakeRegistry(t)
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	testkit.RegisterMounted(t, cat, store)
	return fed{catalog: cat, store: store, publicRepo: publicRepo, registry: registry}
}

func TestT11ResolveView(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineView("alice-default", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main"},
		{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	r1, err := s.catalog.ResolveView("alice-default")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.catalog.ResolveView("alice-default")
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
	_, err := s.catalog.DefineView("dup", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	})
	testkit.ExpectCode(t, err, kernel.ErrViewGenerationInvalid)
	if _, err := s.catalog.DefineView("bad-ref", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/missing"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.catalog.ResolveView("bad-ref")
	testkit.ExpectCode(t, err, kernel.ErrViewGenerationInvalid)
}

func TestT11FederatedReadDoesNotOverride(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineView("v", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	results, err := s.catalog.FederatedRead("v", "policy/P-103")
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
	if _, err := s.catalog.DefineView("v", 1, []catalog.ViewSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	absent, err := s.catalog.FederatedRead("v", "absent")
	if err != nil || len(absent) != 0 {
		t.Fatal(absent, err)
	}
	s.store.Delete("kr://acme/public/core")
	_, err = s.catalog.FederatedRead("v", "absent")
	testkit.ExpectCode(t, err, kernel.ErrViewGenerationInvalid)
}

func TestT11ReadViewFollowsBranch(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineView("v", 1, []catalog.ViewSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	first, err := s.catalog.FederatedRead("v", "policy/P-103")
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	if first[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal(first[0].Value)
	}
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.publicRepo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"statement": "later"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	next, err := s.catalog.FederatedRead("v", "policy/P-103")
	if err != nil || next[0].Value.(map[string]any)["statement"] != "later" {
		t.Fatal(next, err)
	}
	_, err = s.catalog.FederatedRead("missing", "policy/P-103")
	testkit.ExpectCode(t, err, kernel.ErrViewGenerationInvalid)
}

func TestT11NilRegistryRejected(t *testing.T) {
	_, err := catalog.NewCatalog(repository.NewStore(), nil)
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)
}

func TestT11RegistrySurvives(t *testing.T) {
	s := setupFed(t)
	cat := s.catalog
	if _, err := cat.DefineView("v", 1, []catalog.ViewSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	again, err := catalog.NewCatalog(s.store, s.registry)
	if err != nil {
		t.Fatal(err)
	}
	read, err := again.FederatedRead("v", "policy/P-103")
	if err != nil || read[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal(read, err)
	}
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.publicRepo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"statement": "later"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	read, err = again.FederatedRead("v", "policy/P-103")
	if err != nil || read[0].Value.(map[string]any)["statement"] != "later" {
		t.Fatal(read, err)
	}
}

func TestT11GitRegistryHistory(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineView("v", 1, []catalog.ViewSource{{Repository: "kr://acme/public/core", Selector: "refs/heads/main"}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.catalog.ResolveView("v")
	if err != nil {
		t.Fatal(err)
	}
	check := s.catalog.CheckResolved(resolved)
	if check.Outcome != "PASSED" {
		t.Fatal(check)
	}
	again, err := catalog.NewCatalog(s.store, s.registry)
	if err != nil {
		t.Fatal(err)
	}
	viewLog := again.Log(catalog.CatalogLogQuery{View: "v", Limit: 20}).Commits
	sawDefine := false
	for _, item := range viewLog {
		if strings.HasPrefix(item.Message, "define-view") {
			sawDefine = true
		}
	}
	if !sawDefine {
		t.Fatal(viewLog)
	}
	s.store.Delete("kr://acme/public/core")
	failed := again.CheckResolved(resolved)
	if failed.Outcome != "FAILED" || failed.Issues[0].Code != kernel.ErrTemporaryUnavailable {
		t.Fatal(failed)
	}
}
