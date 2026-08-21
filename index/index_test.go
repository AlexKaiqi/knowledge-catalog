package index_test

import (
	"strings"
	"testing"

	"kc/catalog"
	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

func putAt(t *testing.T, repo *local.FileGitRepository, base kernel.CommitID, ops []repository.Operation) kernel.CommitID {
	t.Helper()
	head, err := repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func policyBodySchema() repository.Operation {
	return repository.Operation{
		Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/policy.body"},
		Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
		},
	}
}

func TestIndexIncrementalOnObjectChange(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []repository.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	first, err := idx.Apply(repo, root, c1, []kernel.ObjectID{"policy/P-1"})
	if err != nil || first.Mode != index.IndexModeRebuild || first.Cause != index.IndexCauseCold {
		t.Fatalf("%#v %v", first, err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-2", map[string]any{"body": "another runbook"}, ""))
	second, err := idx.Apply(repo, c1, c2, []kernel.ObjectID{"policy/P-2"})
	if err != nil || second.Mode != index.IndexModeIncremental || second.Cause != index.IndexCauseContent || second.Updated != 1 {
		t.Fatalf("want content incremental, got %#v %v", second, err)
	}
	hits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(hits) != 2 {
		t.Fatalf("%d %v", len(hits), err)
	}
}

func TestIndexRemoveIsIncremental(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []repository.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/gone", map[string]any{"body": "temporary runbook"}, "")[0],
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	c2 := putAt(t, repo, c1, []repository.Operation{{
		Op: repository.OpRemove, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "policy/gone"},
	}})
	sync, err := idx.Apply(repo, c1, c2, []kernel.ObjectID{"policy/gone"})
	if err != nil || sync.Mode != index.IndexModeIncremental || sync.Cause != index.IndexCauseContent || sync.Removed != 1 {
		t.Fatalf("%#v %v", sync, err)
	}
	hits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(hits) != 0 {
		t.Fatalf("%#v %v", hits, err)
	}
}

func TestIndexSchemaChangeForcesRebuild(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{"db": map[string]any{"access": []any{"filter"}}},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl", "note": "events"}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	first, err := idx.Rebuild(repo, c1)
	if err != nil {
		t.Fatal(err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("schema/dw.table.structure", map[string]any{
		"entity": "Table", "aspect": "structure", "pattern": "record",
		"fields": map[string]any{
			"db":   map[string]any{"access": []any{"filter"}},
			"note": map[string]any{"access": []any{"text"}},
		},
	}, ""))
	second, err := idx.Apply(repo, c1, c2, []kernel.ObjectID{"schema/dw.table.structure"})
	if err != nil || second.Mode != index.IndexModeRebuild || second.Cause != index.IndexCauseSchema {
		t.Fatalf("AccessHints change must rebuild cause=schema, got %#v %v", second, err)
	}
	if first.SchemaDigest == second.SchemaDigest {
		t.Fatal("digest must change")
	}
	hits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("events"), reader.SearchEQ("db", "tl")))
	if err != nil || len(hits) != 1 {
		t.Fatalf("%#v %v", hits, err)
	}
}

func TestIndexSchemaDocWithoutHintChangeIsContent(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	schema := map[string]any{
		"entity": "Table", "aspect": "structure", "pattern": "record",
		"title":  "v1",
		"fields": map[string]any{"db": map[string]any{"access": []any{"filter"}}},
	}
	c1 := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: schema},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	schema["title"] = "v2"
	c2 := putAt(t, repo, c1, testkit.PutEntity("schema/dw.table.structure", schema, ""))
	sync, err := idx.Apply(repo, c1, c2, []kernel.ObjectID{"schema/dw.table.structure"})
	if err != nil || sync.Mode != index.IndexModeIncremental || sync.Cause != index.IndexCauseContent {
		t.Fatalf("schema object edit without AccessHints change is content: %#v %v", sync, err)
	}
}

func TestIndexOmitsUnhintedPermissions(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"db":          map[string]any{"access": []any{"filter"}},
				"description": map[string]any{"access": []any{"text"}},
			},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.permissions"}, Value: map[string]any{
			"entity": "Table", "aspect": "permissions", "pattern": "record",
			"fields": map[string]any{"principal": map[string]any{"type": "string"}},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl", "description": "user events"}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "permissions"}, Value: map[string]any{"principal": "user:a", "privileges": []any{"SELECT"}}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	ok, err := idx.Search(repo, reader.SearchOf(reader.SearchEQ("db", "tl")))
	if err != nil || len(ok) != 1 {
		t.Fatalf("%#v %v", ok, err)
	}
	acl, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("SELECT")))
	if err != nil || len(acl) != 0 {
		t.Fatalf("unhinted GRANT body must not be a text document: %#v %v", acl, err)
	}
}

func TestIndexPermissionsWhenHinted(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{"db": map[string]any{"access": []any{"filter"}}},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.permissions"}, Value: map[string]any{
			"entity": "Table", "aspect": "permissions", "pattern": "record",
			"fields": map[string]any{"principal": map[string]any{"access": []any{"filter"}}},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "permissions"}, Value: map[string]any{"principal": "user:a"}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.Search(repo, reader.SearchOf(reader.SearchEQ("principal", "user:a")))
	if err != nil || len(hits) != 1 || hits[0].Address.ObjectID != "Table:t" {
		t.Fatalf("hinted permissions are knowledge: %#v %v", hits, err)
	}
	miss, err := idx.Search(repo, reader.SearchOf(reader.SearchEQ("principal", "user:b")))
	if err != nil || len(miss) != 0 {
		t.Fatalf("%#v %v", miss, err)
	}
}

func TestCatalogHookUpdatesIndexAfterCommit(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cat := testkit.OpenCatalog(t, s.Store)
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	cat.AddHook(&indexHook{idx: idx})

	if _, err := s.Writer.Commit("w0", repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{policyBodySchema()},
	}); err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, s.Repo, "")
	if _, err := s.Writer.Commit("w1", repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-1", map[string]any{"body": "owned runbook"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	head = testkit.MustHead(t, s.Repo, "")
	if _, err := s.Writer.Commit("w2", repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-2", map[string]any{"body": "second runbook"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.Search(s.Repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(hits) != 2 {
		t.Fatalf("%d %v", len(hits), err)
	}
	desc, err := idx.Describe(s.Repo)
	if err != nil || desc.LagBehindHead {
		t.Fatalf("%#v %v", desc, err)
	}
}

func TestProposalDoesNotNotifyCatalog(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cat := testkit.OpenCatalog(t, s.Store)
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	var snaps int
	cat.AddHook(&countSnap{n: &snaps, next: &indexHook{idx: idx}})
	if _, err := s.Writer.Propose("p1", writer.ProposeIntent{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
		CandidateRef:     "refs/kc/proposals/p1",
		Operations:       testkit.PutEntity("policy/P-9", map[string]any{"body": "candidate only"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	if snaps != 0 {
		t.Fatalf("proposal must not NotifySnapshot: %d", snaps)
	}
}

func TestIndexUndeclaredMatchIsUnsatisfied(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, ""))
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	_, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("undeclared MATCH must not dump JSON: %v", err)
	}
}

func TestIndexSchemaRefSelectsFields(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/alpha"}, Value: map[string]any{
			"entity": "Doc", "pattern": "record",
			"fields": map[string]any{"secret": map[string]any{"access": []any{"text"}}},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/beta"}, Value: map[string]any{
			"entity": "Doc", "pattern": "record",
			"fields": map[string]any{"note": map[string]any{"access": []any{"text"}}},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "Doc:a"},
			Value: map[string]any{"secret": "alphasecret", "note": "betanote"}, SchemaRef: "schema/alpha"},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "Doc:b"},
			Value: map[string]any{"secret": "alphasecret", "note": "betanote"}, SchemaRef: "schema/beta"},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	alpha, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("alphasecret")))
	if err != nil || len(alpha) != 1 || string(alpha[0].Address.ObjectID) != "Doc:a" {
		t.Fatalf("alpha: %#v %v", objectIDs(alpha), err)
	}
	beta, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("betanote")))
	if err != nil || len(beta) != 1 || string(beta[0].Address.ObjectID) != "Doc:b" {
		t.Fatalf("beta: %#v %v", objectIDs(beta), err)
	}
}

func TestDescribeIndexShowsCompiledSpec(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []repository.Operation{
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"db":   map[string]any{"type": "string", "access": []any{"filter"}},
				"note": map[string]any{"access": []any{"text"}},
			},
		}},
		{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:t", AspectName: "structure"},
			Value: map[string]any{"db": "tl", "note": "events"}},
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	desc, err := idx.Describe(repo)
	if err != nil || desc.ObjectCount != 1 {
		t.Fatalf("schema objects must not count as documents: %#v %v", desc, err)
	}
	if len(desc.Fields) != 2 || desc.SchemaDigest == "" {
		t.Fatalf("%#v", desc)
	}
	lanes := strings.Join(desc.Lanes, ",")
	if !strings.Contains(lanes, "filter") || !strings.Contains(lanes, "text") {
		t.Fatalf("lanes %v", desc.Lanes)
	}
}

func TestSearchAtDoesNotRewindLive(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []repository.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	idx := index.NewIndexEngine("", local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Ensure(repo, c1); err != nil {
		t.Fatal(err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-1", map[string]any{"body": "later live"}, ""))
	if _, err := idx.Ensure(repo, c2); err != nil {
		t.Fatal(err)
	}

	pinHits, err := idx.SearchAt(repo, c1, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(pinHits) != 1 {
		t.Fatalf("pin search: %#v %v", pinHits, err)
	}
	if pinHits[0].Commit != c1 {
		t.Fatalf("hydrate must use the pin, got %s", pinHits[0].Commit)
	}
	laterOnPin, err := idx.SearchAt(repo, c1, reader.SearchOf(reader.SearchMATCH("later")))
	if err != nil || len(laterOnPin) != 0 {
		t.Fatalf("pin must not see live text: %#v %v", laterOnPin, err)
	}

	liveHits, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("later")))
	if err != nil || len(liveHits) != 1 {
		t.Fatalf("live search: %#v %v", liveHits, err)
	}
	runbookLive, err := idx.Search(repo, reader.SearchOf(reader.SearchMATCH("runbook")))
	if err != nil || len(runbookLive) != 0 {
		t.Fatalf("live must stay on HEAD: %#v %v", runbookLive, err)
	}
	desc, err := idx.Describe(repo)
	if err != nil || desc.BasisCommit != c2 {
		t.Fatalf("SearchAt must not rewind live basis: %#v %v", desc, err)
	}
}

type indexHook struct{ idx *index.Index }

func (h *indexHook) AfterSnapshot(ev catalog.Snapshot) error {
	return h.idx.AfterSnapshot(ev.Repository, ev.From, ev.To, ev.ObjectIDs)
}

type countSnap struct {
	n    *int
	next catalog.Hook
}

func (c *countSnap) AfterSnapshot(ev catalog.Snapshot) error {
	*c.n++
	if c.next != nil {
		return c.next.AfterSnapshot(ev)
	}
	return nil
}
