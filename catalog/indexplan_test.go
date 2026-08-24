package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
)

func schemaDoc(entity, aspect string, fields map[string]any) map[string]any {
	return map[string]any{
		"entity": entity, "aspect": aspect, "pattern": "record", "fields": fields,
	}
}

func commitSchema(t *testing.T, repo *local.FileGitRepository, objects map[string]any) kernel.CommitID {
	t.Helper()
	head := testkit.MustHead(t, repo, "refs/heads/main")
	var ops []repository.Operation
	for id, value := range objects {
		ops = append(ops, repository.Operation{
			Op:      repository.OpPut,
			Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: kernel.ObjectID(id)},
			Value:   value,
		})
	}
	head, err := repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestPlanIndexFromView(t *testing.T) {
	public := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, public, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db":          map[string]any{"access": []any{"filter"}},
			"description": map[string]any{"access": []any{"text", "summary"}},
		}),
		"schema/dw.table.permissions": schemaDoc("Table", "permissions", map[string]any{
			"principal": map[string]any{"type": "string"},
		}),
	})
	store := repository.NewStore()
	if err := store.Add(public); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("physical", 1, []catalog.WorkspaceSource{
		{Repository: public.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := testkit.PlanIndex(cat, "physical")
	if err != nil {
		t.Fatal(err)
	}
	if plan.WorkspaceID != "physical" || len(plan.Projections) != 1 {
		t.Fatalf("%#v", plan)
	}
	proj := plan.Projections[0]
	resolved, err := cat.ResolveWorkspace("physical")
	if err != nil {
		t.Fatal(err)
	}
	if proj.Repository != public.ID() || proj.Commit != resolved.Repositories[public.ID()] {
		t.Fatalf("%#v", proj)
	}
	if len(proj.Fields) != 2 {
		t.Fatalf("unhinted permissions must stay out: %#v", proj.Fields)
	}
	if len(proj.Lanes) != 2 || proj.Lanes[0] != reader.LaneFilter || proj.Lanes[1] != reader.LaneText {
		t.Fatalf("lanes %#v", proj.Lanes)
	}
	for _, field := range proj.Fields {
		if field.Aspect == "permissions" {
			t.Fatal(field)
		}
		if field.Path == "description" && !containsHint(field.Access, reader.HintSummary) {
			t.Fatalf("summary is stored on the field, not a lane: %#v", field)
		}
	}
	if proj.SchemaDigest == "" {
		t.Fatal("schema digest required")
	}
}

func TestPlanIndexIncludesHintedPermissions(t *testing.T) {
	public := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, public, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"access": []any{"filter"}},
		}),
		"schema/dw.table.permissions": schemaDoc("Table", "permissions", map[string]any{
			"principal": map[string]any{"access": []any{"filter"}},
		}),
	})
	store := repository.NewStore()
	if err := store.Add(public); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("physical", 1, []catalog.WorkspaceSource{
		{Repository: public.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := testkit.PlanIndex(cat, "physical")
	if err != nil {
		t.Fatal(err)
	}
	proj := plan.Projections[0]
	var sawPermissions bool
	for _, field := range proj.Fields {
		if field.Aspect == "permissions" && field.Path == "principal" {
			sawPermissions = true
		}
	}
	if !sawPermissions || len(proj.Fields) != 2 {
		t.Fatalf("hinted permissions follow AccessHints: %#v", proj.Fields)
	}
}

func TestPlanIndexTwoRepositories(t *testing.T) {
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
	store := repository.NewStore()
	for _, repo := range []*local.FileGitRepository{physical, semantic} {
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
	plan, err := testkit.PlanIndex(cat, "both")
	if err != nil || len(plan.Projections) != 2 {
		t.Fatalf("%#v %v", plan, err)
	}
	if plan.Projections[0].Repository != physical.ID() || plan.Projections[1].Repository != semantic.ID() {
		t.Fatalf("must sort by repository: %#v", plan.Projections)
	}
	if len(plan.Projections[0].Lanes) != 1 || plan.Projections[0].Lanes[0] != reader.LaneFilter {
		t.Fatalf("%#v", plan.Projections[0])
	}
	if len(plan.Projections[1].Lanes) != 1 || plan.Projections[1].Lanes[0] != reader.LaneText {
		t.Fatalf("%#v", plan.Projections[1])
	}
}

func TestPlanIndexDigestChangesWithHints(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, repo, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"access": []any{"filter"}},
		}),
	})
	store := repository.NewStore()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	p1, err := testkit.PlanIndex(cat, "v")
	if err != nil {
		t.Fatal(err)
	}
	c1 := p1.Projections[0].Commit
	commitSchema(t, repo, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db":   map[string]any{"access": []any{"filter"}},
			"body": map[string]any{"access": []any{"text"}},
		}),
	})
	p2, err := testkit.PlanIndex(cat, "v")
	if err != nil {
		t.Fatal(err)
	}
	if p1.Projections[0].Commit == p2.Projections[0].Commit {
		t.Fatal("schema commit must move the resolved commit", c1)
	}
	if p1.Projections[0].SchemaDigest == p2.Projections[0].SchemaDigest {
		t.Fatal("hint change must change schemaDigest")
	}
}

func TestPlanIndexUnknownView(t *testing.T) {
	s := setupFed(t)
	_, err := testkit.PlanIndex(s.catalog, "missing")
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
