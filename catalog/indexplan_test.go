package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
	"kc/snapshot/filegit"
)

func schemaDoc(entity, aspect string, fields map[string]any) map[string]any {
	return map[string]any{
		"entity": entity, "aspect": aspect, "pattern": "record", "fields": fields,
	}
}

func commitSchema(t *testing.T, repo *filegit.FileGitRepository, objects map[string]any) kernel.CommitID {
	t.Helper()
	head := testkit.MustHead(t, repo, "refs/heads/main")
	var ops []knowledge.Operation
	for id, value := range objects {
		ops = append(ops, knowledge.Operation{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(id)},
			Value:   value,
		})
	}
	head, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestPlanAccessFromWorkspace(t *testing.T) {
	public := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, public, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db":          map[string]any{"access": []any{"filter"}},
			"description": map[string]any{"access": []any{"text"}},
		}),
		"schema/dw.table.permissions": schemaDoc("Table", "permissions", map[string]any{
			"principal": map[string]any{"type": "string"},
		}),
	})
	store := snapshot.NewRegistry()
	if err := store.Add(public); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("physical", 1, []catalog.WorkspaceSource{
		{Repository: public.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := testkit.PlanAccess(cat, "physical")
	if err != nil {
		t.Fatal(err)
	}
	if plan.WorkspaceID != "physical" || len(plan.Specs) != 1 {
		t.Fatalf("%#v", plan)
	}
	spec := plan.Specs[0]
	resolved, err := cat.ResolveWorkspace("physical")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Repository != public.ID() || spec.Commit != resolved.Repositories[public.ID()] {
		t.Fatalf("%#v", spec)
	}
	if len(spec.Fields) != 2 {
		t.Fatalf("unhinted permissions must stay out: %#v", spec.Fields)
	}
	lanes := spec.QueryLanes()
	if len(lanes) != 2 || lanes[0] != "filter" || lanes[1] != "text" {
		t.Fatalf("lanes %#v", lanes)
	}
	for _, field := range spec.Fields {
		if field.Aspect == "permissions" {
			t.Fatal(field)
		}
	}
	if spec.AccessDigest == "" {
		t.Fatal("access digest required")
	}
}

func TestPlanAccessIncludesHintedPermissions(t *testing.T) {
	public := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, public, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"access": []any{"filter"}},
		}),
		"schema/dw.table.permissions": schemaDoc("Table", "permissions", map[string]any{
			"principal": map[string]any{"access": []any{"filter"}},
		}),
	})
	store := snapshot.NewRegistry()
	if err := store.Add(public); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("physical", 1, []catalog.WorkspaceSource{
		{Repository: public.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := testkit.PlanAccess(cat, "physical")
	if err != nil {
		t.Fatal(err)
	}
	spec := plan.Specs[0]
	var sawPermissions bool
	for _, field := range spec.Fields {
		if field.Aspect == "permissions" && field.Path == "principal" {
			sawPermissions = true
		}
	}
	if !sawPermissions || len(spec.Fields) != 2 {
		t.Fatalf("hinted permissions follow AccessHints: %#v", spec.Fields)
	}
}

func TestPlanAccessTwoRepositories(t *testing.T) {
	physical := testkit.MakeRepository(t, "kr://acme/public/physical")
	semantic := testkit.MakeRepository(t, "kr://acme/public/semantic")
	commitSchema(t, physical, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"access": []any{"filter"}},
		}),
	})
	commitSchema(t, semantic, map[string]any{
		"schema/dw.metric.definition": schemaDoc("Metric", "definition", map[string]any{
			"expr": map[string]any{"access": []any{"text"}},
		}),
	})
	store := snapshot.NewRegistry()
	for _, repo := range []*filegit.FileGitRepository{physical, semantic} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("both", 1, []catalog.WorkspaceSource{
		{Repository: physical.ID(), Selector: "refs/heads/main"},
		{Repository: semantic.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := testkit.PlanAccess(cat, "both")
	if err != nil || len(plan.Specs) != 2 {
		t.Fatalf("%#v %v", plan, err)
	}
	if plan.Specs[0].Repository != physical.ID() || plan.Specs[1].Repository != semantic.ID() {
		t.Fatalf("must sort by repository: %#v", plan.Specs)
	}
	if lanes := plan.Specs[0].QueryLanes(); len(lanes) != 1 || lanes[0] != "filter" {
		t.Fatalf("%#v", plan.Specs[0])
	}
	if lanes := plan.Specs[1].QueryLanes(); len(lanes) != 1 || lanes[0] != "text" {
		t.Fatalf("%#v", plan.Specs[1])
	}
}

func TestPlanAccessDigestChangesWithHints(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, repo, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"access": []any{"filter"}},
		}),
	})
	store := snapshot.NewRegistry()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	p1, err := testkit.PlanAccess(cat, "v")
	if err != nil {
		t.Fatal(err)
	}
	c1 := p1.Specs[0].Commit
	commitSchema(t, repo, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db":   map[string]any{"access": []any{"filter"}},
			"body": map[string]any{"access": []any{"text"}},
		}),
	})
	p2, err := testkit.PlanAccess(cat, "v")
	if err != nil {
		t.Fatal(err)
	}
	if p1.Specs[0].Commit == p2.Specs[0].Commit {
		t.Fatal("schema commit must move the resolved commit", c1)
	}
	if p1.Specs[0].AccessDigest == p2.Specs[0].AccessDigest {
		t.Fatal("hint change must change accessDigest")
	}
}

func TestPlanAccessUnknownWorkspace(t *testing.T) {
	s := setupFed(t)
	_, err := testkit.PlanAccess(s.catalog, "missing")
	testkit.ExpectCode(t, err, kernel.ErrWorkspaceInvalid)
}

func containsHint(got []reader.AccessHint, want reader.AccessHint) bool {
	for _, h := range got {
		if h == want {
			return true
		}
	}
	return false
}
