package index_test

import (
	"kc/retrieval"
	"strconv"
	"strings"
	"testing"

	"kc/catalog"
	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type staleCandidateEngine struct {
	meta       index.Meta
	candidates []index.CandidateRef
}

type nonAdvancingEngine struct {
	staleCandidateEngine
	calls int
}

type supersetEngine struct {
	meta index.Meta
	ids  []knowledge.ObjectID
}

func (e *supersetEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeSuperset, Coverage: 1}
}
func (e *supersetEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	offset := 0
	if req.Continuation != "" {
		offset, _ = strconv.Atoi(req.Continuation)
	}
	limit := req.Search.Limit
	if limit <= 0 || offset+limit > len(e.ids) {
		limit = len(e.ids) - offset
	}
	page := index.CandidatePage{Exhausted: offset+limit >= len(e.ids)}
	for _, id := range e.ids[offset : offset+limit] {
		page.Candidates = append(page.Candidates, index.CandidateRef{ObjectID: id, Basis: e.meta.Basis})
	}
	if !page.Exhausted {
		page.Continuation = strconv.Itoa(offset + limit)
	}
	return page, nil
}
func (e *supersetEngine) LoadMeta() (index.Meta, error) { return e.meta, nil }
func (e *supersetEngine) Rebuild(_ []index.CompiledDoc, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *supersetEngine) Apply(_ []index.CompiledDoc, _ []knowledge.ObjectID, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *supersetEngine) Count() (int, error) { return len(e.ids), nil }
func (e *supersetEngine) Close() error        { return nil }

func (e *staleCandidateEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}
func (e *staleCandidateEngine) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	if e.candidates != nil {
		return index.CandidatePage{Candidates: append([]index.CandidateRef(nil), e.candidates...), Exhausted: true}, nil
	}
	return index.CandidatePage{Candidates: []index.CandidateRef{
		{ObjectID: "policy/P-1", Basis: e.meta.Basis},
		{ObjectID: "policy/removed", Basis: e.meta.Basis},
		{ObjectID: "policy/wrong-basis", Basis: "stale-commit"},
	}, Exhausted: true}, nil
}
func (e *staleCandidateEngine) LoadMeta() (index.Meta, error) { return e.meta, nil }
func (e *staleCandidateEngine) Rebuild(_ []index.CompiledDoc, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *staleCandidateEngine) Apply(_ []index.CompiledDoc, _ []knowledge.ObjectID, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *staleCandidateEngine) Count() (int, error) { return 3, nil }
func (e *staleCandidateEngine) Close() error        { return nil }

func (e *nonAdvancingEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	e.calls++
	if e.calls == 1 {
		return index.CandidatePage{Continuation: "stuck"}, nil
	}
	return index.CandidatePage{Continuation: req.Continuation}, nil
}

type knowledgeWriter interface {
	knowledge.Repository
	ApplyKnowledgeCommit(knowledge.ChangeSet) (kernel.CommitID, error)
}

func putAt(t *testing.T, repo knowledgeWriter, base kernel.CommitID, ops []knowledge.Operation) kernel.CommitID {
	t.Helper()
	head, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func policyBodySchema() knowledge.Operation {
	return knowledge.Operation{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"},
		Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
		},
	}
}

func TestIndexIncrementalOnObjectChange(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	first, err := idx.Apply(repo, root, c1, []knowledge.ObjectID{"policy/P-1"})
	if err != nil || first.Mode != index.IndexModeRebuild || first.Cause != index.IndexCauseCold {
		t.Fatalf("%#v %v", first, err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-2", map[string]any{"body": "another runbook"}, ""))
	second, err := idx.Apply(repo, c1, c2, []knowledge.ObjectID{"policy/P-2"})
	if err != nil || second.Mode != index.IndexModeIncremental || second.Cause != index.IndexCauseContent || second.Updated != 1 {
		t.Fatalf("want content incremental, got %#v %v", second, err)
	}
	hits, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 2 {
		t.Fatalf("%d %v", len(hits.Hits), err)
	}
}

func TestSearchRejectsCandidateMissingFromFixedAuthorityBasis(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "runbook"}, "")[0],
	})
	engine := &staleCandidateEngine{candidates: []index.CandidateRef{
		{ObjectID: "policy/P-1", Basis: head},
		{ObjectID: "policy/removed", Basis: head},
	}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	_, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("missing fixed-basis candidate must be a consistency error: %v", err)
	}
}

func TestSearchRejectsNonAdvancingProviderContinuation(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/non-advancing-search")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit := putAt(t, repo, root, []knowledge.Operation{policyBodySchema()})
	engine := &nonAdvancingEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	_, err := idx.SearchAt(repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("non-advancing continuation must fail closed: %v", err)
	}
	if engine.calls != 2 {
		t.Fatalf("provider must be stopped at the first repeated continuation: calls=%d", engine.calls)
	}
}

func TestSupersetResidualRestoresCompleteAndContinuesCandidates(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/residual")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "fields": map[string]any{"db": map[string]any{"type": "string", "access": []any{"filter"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:no", AspectName: "structure"}, Value: map[string]any{"db": "other"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:a", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:b", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
	})
	engine := &supersetEngine{ids: []knowledge.ObjectID{"Table:no", "Table:a", "Table:b"}}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	request := retrieval.SearchOf(retrieval.SearchEQ("db", "tl"))
	request.Limit = 1
	first, err := idx.SearchAt(repo, head, request)
	if err != nil || first.Completeness != retrieval.CompletenessComplete || len(first.Hits) != 1 || first.Hits[0].Knowledge.Address.ObjectID != "Table:a" || first.Continuation == "" {
		t.Fatalf("first residual page: %#v %v", first, err)
	}
	request.Continuation = first.Continuation
	second, err := idx.SearchAt(repo, head, request)
	if err != nil || second.Completeness != retrieval.CompletenessComplete || len(second.Hits) != 1 || second.Hits[0].Knowledge.Address.ObjectID != "Table:b" || second.Continuation != "" {
		t.Fatalf("second residual page: %#v %v", second, err)
	}
}

func TestIndexRemoveIsIncremental(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/gone", map[string]any{"body": "temporary runbook"}, "")[0],
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	c2 := putAt(t, repo, c1, []knowledge.Operation{{
		Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/gone"},
	}})
	sync, err := idx.Apply(repo, c1, c2, []knowledge.ObjectID{"policy/gone"})
	if err != nil || sync.Mode != index.IndexModeIncremental || sync.Cause != index.IndexCauseContent || sync.Removed != 1 {
		t.Fatalf("%#v %v", sync, err)
	}
	hits, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 0 {
		t.Fatalf("%#v %v", hits, err)
	}
}

func TestIndexSchemaChangeForcesRebuild(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{"db": map[string]any{"access": []any{"filter"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl", "note": "events"}},
	})
	idx := liveIndex(t)
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
	second, err := idx.Apply(repo, c1, c2, []knowledge.ObjectID{"schema/dw.table.structure"})
	if err != nil || second.Mode != index.IndexModeRebuild || second.Cause != index.IndexCauseSchema {
		t.Fatalf("AccessHints change must rebuild cause=schema, got %#v %v", second, err)
	}
	if first.AccessDigest == second.AccessDigest {
		t.Fatal("digest must change")
	}
	hits, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("events"), retrieval.SearchEQ("db", "tl")))
	if err != nil || len(hits.Hits) != 1 {
		t.Fatalf("%#v %v", hits, err)
	}
}

func TestIndexSchemaDocWithoutHintChangeIsContent(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	schema := map[string]any{
		"entity": "Table", "aspect": "structure", "pattern": "record",
		"title":  "v1",
		"fields": map[string]any{"db": map[string]any{"access": []any{"filter"}}},
	}
	c1 := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: schema},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	schema["title"] = "v2"
	c2 := putAt(t, repo, c1, testkit.PutEntity("schema/dw.table.structure", schema, ""))
	sync, err := idx.Apply(repo, c1, c2, []knowledge.ObjectID{"schema/dw.table.structure"})
	if err != nil || sync.Mode != index.IndexModeIncremental || sync.Cause != index.IndexCauseContent {
		t.Fatalf("schema object edit without AccessHints change is content: %#v %v", sync, err)
	}
}

func TestIndexOmitsUnhintedPermissions(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"db":          map[string]any{"access": []any{"filter"}},
				"description": map[string]any{"access": []any{"text"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.permissions"}, Value: map[string]any{
			"entity": "Table", "aspect": "permissions", "pattern": "record",
			"fields": map[string]any{"principal": map[string]any{"type": "string"}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl", "description": "user events"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "permissions"}, Value: map[string]any{"principal": "user:a", "privileges": []any{"SELECT"}}},
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	ok, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchEQ("db", "tl")))
	if err != nil || len(ok.Hits) != 1 {
		t.Fatalf("%#v %v", ok, err)
	}
	acl, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchMATCH("SELECT")))
	if err != nil || len(acl.Hits) != 0 {
		t.Fatalf("unhinted GRANT body must not be a text document: %#v %v", acl, err)
	}
}

func TestIndexPermissionsWhenHinted(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{"db": map[string]any{"access": []any{"filter"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.permissions"}, Value: map[string]any{
			"entity": "Table", "aspect": "permissions", "pattern": "record",
			"fields": map[string]any{"principal": map[string]any{"access": []any{"filter"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "structure"}, Value: map[string]any{"db": "tl"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "permissions"}, Value: map[string]any{"principal": "user:a"}},
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchEQ("principal", "user:a")))
	if err != nil || len(hits.Hits) != 1 || hits.Hits[0].Knowledge.Address.ObjectID != "Table:t" {
		t.Fatalf("hinted permissions are knowledge: %#v %v", hits, err)
	}
	miss, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchEQ("principal", "user:b")))
	if err != nil || len(miss.Hits) != 0 {
		t.Fatalf("%#v %v", miss, err)
	}
}

func TestCatalogHookUpdatesIndexAfterCommit(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cat := testkit.OpenCatalog(t, s.Store)
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	cat.AddHook(&indexHook{idx: idx})

	if _, err := s.Writer.Commit("w0", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{policyBodySchema()},
	}); err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, s.Repo, "")
	if _, err := s.Writer.Commit("w1", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-1", map[string]any{"body": "owned runbook"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	head = testkit.MustHead(t, s.Repo, "")
	if _, err := s.Writer.Commit("w2", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-2", map[string]any{"body": "second runbook"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	head = testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	hits, err := idx.SearchAt(s.Repo, head, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(hits.Hits) != 2 {
		t.Fatalf("%d %v", len(hits.Hits), err)
	}
	desc, err := idx.Describe(s.Repo)
	if err != nil || desc.LagBehindHead {
		t.Fatalf("%#v %v", desc, err)
	}
}

func TestProposalDoesNotNotifyCatalog(t *testing.T) {
	s := testkit.NewSetup(t, "")
	cat := testkit.OpenCatalog(t, s.Store)
	idx := liveIndex(t)
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
	repo := makeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, ""))
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, c1); err != nil {
		t.Fatal(err)
	}
	_, err := idx.SearchAt(repo, c1, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("undeclared MATCH must not dump JSON: %v", err)
	}
}

func TestIndexSchemaRefSelectsFields(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/alpha"}, Value: map[string]any{
			"entity": "Doc", "pattern": "record",
			"fields": map[string]any{"secret": map[string]any{"access": []any{"text"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/beta"}, Value: map[string]any{
			"entity": "Doc", "pattern": "record",
			"fields": map[string]any{"note": map[string]any{"access": []any{"text"}}},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Doc:a"},
			Value: map[string]any{"secret": "alphasecret", "note": "betanote"}, SchemaRef: "schema/alpha"},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "Doc:b"},
			Value: map[string]any{"secret": "alphasecret", "note": "betanote"}, SchemaRef: "schema/beta"},
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	alpha, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchMATCH("alphasecret")))
	if err != nil || len(alpha.Hits) != 1 || string(alpha.Hits[0].Knowledge.Address.ObjectID) != "Doc:a" {
		t.Fatalf("alpha: %#v %v", alpha.Hits, err)
	}
	beta, err := idx.SearchAt(repo, head, retrieval.SearchOf(retrieval.SearchMATCH("betanote")))
	if err != nil || len(beta.Hits) != 1 || string(beta.Hits[0].Knowledge.Address.ObjectID) != "Doc:b" {
		t.Fatalf("beta: %#v %v", beta.Hits, err)
	}
}

func TestDescribeIndexShowsCompiledSpec(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/physical")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	head := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/dw.table.structure"}, Value: map[string]any{
			"entity": "Table", "aspect": "structure", "pattern": "record",
			"fields": map[string]any{
				"db":   map[string]any{"type": "string", "access": []any{"filter"}},
				"note": map[string]any{"access": []any{"text"}},
			},
		}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Table:t", AspectName: "structure"},
			Value: map[string]any{"db": "tl", "note": "events"}},
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, head); err != nil {
		t.Fatal(err)
	}
	desc, err := idx.Describe(repo)
	if err != nil || desc.ObjectCount != 1 {
		t.Fatalf("schema objects must not count as documents: %#v %v", desc, err)
	}
	if len(desc.Fields) != 2 || desc.AccessDigest == "" {
		t.Fatalf("%#v", desc)
	}
	lanes := strings.Join(desc.Lanes, ",")
	if !strings.Contains(lanes, "filter") || !strings.Contains(lanes, "text") {
		t.Fatalf("lanes %v", desc.Lanes)
	}
}

func TestSearchAtDoesNotRewindLive(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []knowledge.Operation{
		policyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Ensure(repo, c1); err != nil {
		t.Fatal(err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("policy/P-1", map[string]any{"body": "later live"}, ""))
	if _, err := idx.Ensure(repo, c2); err != nil {
		t.Fatal(err)
	}
	// Consumer reads are deliberately read-only. Operations must publish the
	// frozen projection before SearchAt can serve the old task basis.
	if _, err := idx.EnsureAt(repo, c1); err != nil {
		t.Fatal(err)
	}

	pinHits, err := idx.SearchAt(repo, c1, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(pinHits.Hits) != 1 {
		t.Fatalf("pin search: %#v %v", pinHits, err)
	}
	if pinHits.Hits[0].Knowledge.Commit != c1 {
		t.Fatalf("hydrate must use the pin, got %s", pinHits.Hits[0].Knowledge.Commit)
	}
	laterOnPin, err := idx.SearchAt(repo, c1, retrieval.SearchOf(retrieval.SearchMATCH("later")))
	if err != nil || len(laterOnPin.Hits) != 0 {
		t.Fatalf("pin must not see live text: %#v %v", laterOnPin, err)
	}

	liveHits, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("later")))
	if err != nil || len(liveHits.Hits) != 1 {
		t.Fatalf("live search: %#v %v", liveHits, err)
	}
	runbookLive, err := idx.SearchAt(repo, c2, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(runbookLive.Hits) != 0 {
		t.Fatalf("live must stay on HEAD: %#v %v", runbookLive, err)
	}
	desc, err := idx.Describe(repo)
	if desc.BasisCommit != c2 {
		t.Fatalf("SearchAt must not rewind live basis: %#v %v", desc, err)
	}
}

func TestIndexSharedPathUntypedObjectKeepsLiveAtHead(t *testing.T) {
	repo := makeIndexRepository(t, "kr://acme/org/semantics")
	root := testkit.MustHead(t, repo, "refs/heads/main")
	c1 := putAt(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/skill.body"},
			Value: map[string]any{"entity": "Skill", "pattern": "record", "fields": map[string]any{"text": map[string]any{"access": []any{"text"}}}}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/metricview.definition"},
			Value: map[string]any{"entity": "MetricView", "pattern": "record", "fields": map[string]any{"text": map[string]any{"access": []any{"text"}}}}},
		testkit.PutEntity("Skill:sql.execute", map[string]any{"text": "run sql"}, "")[0],
	})
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Ensure(repo, c1); err != nil {
		t.Fatal(err)
	}
	c2 := putAt(t, repo, c1, testkit.PutEntity("Note:channel", map[string]any{"text": "semantic o_channel"}, ""))
	if _, err := idx.Ensure(repo, c2); err != nil {
		t.Fatal(err)
	}
	c3 := putAt(t, repo, c2, testkit.PutEntity("Metric:idem", map[string]any{"description": "probe", "formula": "1"}, ""))
	if _, err := idx.Ensure(repo, c3); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.EnsureAt(repo, c2); err != nil {
		t.Fatal(err)
	}
	desc, err := idx.Describe(repo)
	if err != nil || desc.LagBehindHead || desc.BasisCommit != c3 {
		t.Fatalf("untyped note sharing path text must not stall live: %#v %v", desc, err)
	}
	hits, err := idx.SearchAt(repo, c3, retrieval.SearchOf(retrieval.SearchMATCH("semantic")))
	if err != nil || len(hits.Hits) != 1 || hits.Hits[0].Knowledge.Address.ObjectID != "Note:channel" {
		t.Fatalf("note must be searchable after shared-path compile: %#v %v", hits, err)
	}
	pin, err := idx.DescribeAt(repo, c2)
	if err != nil || pin.BasisCommit != c2 {
		t.Fatalf("DescribeAt pin: %#v %v", pin, err)
	}
	live, err := idx.Describe(repo)
	if err != nil || live.BasisCommit != c3 {
		t.Fatalf("DescribeAt must not rewind live: live %#v pin %#v", live, pin)
	}
}

func TestCatalogHookSharedPathDoesNotLeaveLiveLagging(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/org/semantics")
	cat := testkit.OpenCatalog(t, s.Store)
	idx := liveIndex(t)
	t.Cleanup(func() { _ = idx.Close() })
	cat.AddHook(&indexHook{idx: idx})

	if _, err := s.Writer.Commit("schemas", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/skill.body"},
				Value: map[string]any{"entity": "Skill", "pattern": "record", "fields": map[string]any{"text": map[string]any{"access": []any{"text"}}}}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/metricview.definition"},
				Value: map[string]any{"entity": "MetricView", "pattern": "record", "fields": map[string]any{"text": map[string]any{"access": []any{"text"}}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, s.Repo, "")
	if _, err := s.Writer.Commit("note", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("Note:channel", map[string]any{"text": "semantic o_channel"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	head = testkit.MustHead(t, s.Repo, "")
	if _, err := s.Writer.Commit("idem", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("Metric:idem", map[string]any{"formula": "1"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	desc, err := idx.Describe(s.Repo)
	if err != nil || desc.LagBehindHead {
		t.Fatalf("AfterSnapshot must keep live at HEAD: %#v %v", desc, err)
	}
}

type indexHook struct{ idx *index.Index }

func (h *indexHook) AfterSnapshot(ev catalog.Snapshot) error {
	registry := snapshot.NewRegistry()
	if err := registry.Add(ev.Repository); err != nil {
		return err
	}
	repo, err := reader.NewReader(registry).Require(ev.Repository.ID(), kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return err
	}
	return h.idx.AfterSnapshot(repo, ev.From, ev.To, nil)
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
