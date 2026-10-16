package dataset_test

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func schemaDoc(entity, aspect string, fields map[string]any) map[string]any {
	return map[string]any{
		"entity": entity, "aspect": aspect, "pattern": "record", "fields": fields,
	}
}

func commitSchema(t *testing.T, repo *testkit.ObjectRepository, objects map[string]any, declarations ...objectmodel.Operation) kernel.CommitID {
	t.Helper()
	head := testkit.MustHead(t, repo, "refs/heads/main")
	var ops []objectmodel.Operation
	for id, value := range objects {
		ops = append(ops, objectmodel.Operation{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)},
			Value:   value,
		})
	}
	for _, declaration := range declarations {
		if objectmodel.IsIndexDeclarationRef(declaration.SchemaRef) {
			declaration.Address.ObjectID = objectmodel.ObjectID(testkit.IndexDeclarationObject + "/" + strings.TrimPrefix(string(repo.ID()), "kr://"))
		}
		ops = append(ops, declaration)
	}
	head, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

// planDeclaredAccess supplies the declaration witnesses that explicit index
// maintenance retains, then verifies them through the fixed Dataset scope.
func planDeclaredAccess(cat catalog.Service, setID string) (retrieval.AccessPlan, error) {
	resolved, err := cat.ResolveDataset(setID)
	if err != nil {
		return retrieval.AccessPlan{}, err
	}
	lookup := reader.Lookup(cat.Require)
	witnesses := map[kernel.RepositoryID][]retrieval.SchemaAdoption{}
	for id, commit := range resolved.Repositories {
		repo, err := lookup(id)
		if err != nil {
			return retrieval.AccessPlan{}, err
		}
		spec, err := retrieval.AccessSpecForRepository(repo, commit)
		if err != nil {
			return retrieval.AccessPlan{}, err
		}
		witnesses[id] = spec.AdoptionWitnesses()
	}
	return retrieval.PlanAccess(lookup, testkit.DatasetPin(resolved), func(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
		return retrieval.AccessSpecFromAdoptions(repo, commit, witnesses[repo.ID()])
	})
}

func TestPlanAccessFromWorkspace(t *testing.T) {
	public := testkit.MakeRepository(t, "kr://acme/public/physical")
	commitSchema(t, public, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db":          map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
		}),
		"schema/dw.table.permissions": schemaDoc("Table", "permissions", map[string]any{
			"principal": map[string]any{"type": "string"},
		}),
	}, testkit.IndexField("schema/dw.table.structure", "db", "keyword"),
		testkit.IndexText("schema/dw.table.structure", "description"))
	store := snapshot.NewRegistry()
	if err := store.Add(public); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("physical", 1, []catalog.DatasetSource{
		{Repository: public.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := planDeclaredAccess(cat, "physical")
	if err != nil {
		t.Fatal(err)
	}
	if plan.SetID != "physical" || len(plan.Specs) != 1 {
		t.Fatalf("%#v", plan)
	}
	if len(plan.Items) == 0 {
		t.Fatal("access plan must copy the published file list")
	}
	spec := plan.Specs[0]
	resolved, err := cat.ResolveDataset("physical")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Repository != public.ID() || spec.Commit != resolved.Repositories[public.ID()] {
		t.Fatalf("%#v", spec)
	}
	if len(spec.Fields) != 2 {
		t.Fatalf("undeclared permissions must stay out: %#v", spec.Fields)
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
			"db": map[string]any{"type": "string"},
		}),
		"schema/dw.table.permissions": schemaDoc("Table", "permissions", map[string]any{
			"principal": map[string]any{"type": "string"},
		}),
	}, testkit.IndexField("schema/dw.table.structure", "db", "keyword"),
		testkit.IndexField("schema/dw.table.permissions", "principal", "keyword"))
	store := snapshot.NewRegistry()
	if err := store.Add(public); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("physical", 1, []catalog.DatasetSource{
		{Repository: public.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := planDeclaredAccess(cat, "physical")
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
		t.Fatalf("declared permissions must enter the access plan: %#v", spec.Fields)
	}
}

func TestPlanAccessTwoRepositories(t *testing.T) {
	physical := testkit.MakeRepository(t, "kr://acme/public/physical")
	semantic := testkit.MakeRepository(t, "kr://acme/public/semantic")
	commitSchema(t, physical, map[string]any{
		"schema/dw.table.structure": schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"type": "string"},
		}),
	}, testkit.IndexField("schema/dw.table.structure", "db", "keyword"))
	commitSchema(t, semantic, map[string]any{
		"schema/dw.metric.definition": schemaDoc("Metric", "definition", map[string]any{
			"expr": map[string]any{"type": "string"},
		}),
	}, testkit.IndexText("schema/dw.metric.definition", "expr"))
	store := snapshot.NewRegistry()
	for _, repo := range []*testkit.ObjectRepository{physical, semantic} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("both", 1, []catalog.DatasetSource{
		{Repository: physical.ID(), Selector: "refs/heads/main"},
		{Repository: semantic.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := planDeclaredAccess(cat, "both")
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
			"db":   map[string]any{"type": "string"},
			"body": map[string]any{"type": "string"},
		}),
	}, testkit.IndexField("schema/dw.table.structure", "db", "keyword"))
	store := snapshot.NewRegistry()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("v", 1, []catalog.DatasetSource{
		{Repository: repo.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	p1, err := planDeclaredAccess(cat, "v")
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Specs[0].Fields) != 1 {
		t.Fatalf("only the declared db field must be queryable: %#v", p1.Specs[0].Fields)
	}
	c1 := p1.Specs[0].Commit
	commitSchema(t, repo, nil, testkit.IndexText("schema/dw.table.structure", "body"))
	if _, err := cat.DefineDataset("v", 2, []catalog.DatasetSource{
		{Repository: repo.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	p2, err := planDeclaredAccess(cat, "v")
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Specs[0].Fields) != 2 {
		t.Fatalf("the new body declaration must enter the plan: %#v", p2.Specs[0].Fields)
	}
	if p1.Specs[0].Commit == p2.Specs[0].Commit {
		t.Fatal("declaration commit must move the resolved commit", c1)
	}
	if p1.Specs[0].AccessDigest == p2.Specs[0].AccessDigest {
		t.Fatal("declaration change must change accessDigest")
	}
}

func TestPlanAccessExcludesUnselectedDeclarationWitnesses(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/scoped")
	head := commitSchema(t, repo, nil, objectmodel.Operation{
		Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/dw.table.structure"},
		PathHint: "selected/schema.yaml",
		Value: schemaDoc("Table", "structure", map[string]any{
			"db": map[string]any{"type": "string"},
		}),
	}, testkit.IndexField("schema/dw.table.structure", "db", "keyword"))
	maintained, err := retrieval.AccessSpecForRepository(repo, head)
	if err != nil || len(maintained.Fields) != 1 || len(maintained.Declarations) != 1 {
		t.Fatalf("repository maintenance must retain the declaration witness: %#v %v", maintained, err)
	}
	store := snapshot.NewRegistry()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("schema-only", 1, []catalog.DatasetSource{
		{Repository: repo.ID(), Selector: snapshot.DefaultRef, Path: catalog.MountPath(""), SubPath: "selected"},
	}); err != nil {
		t.Fatal(err)
	}
	serving, err := testkit.OpenDataset(cat, "schema-only")
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := serving.Member(repo.ID())
	if err != nil {
		t.Fatal(err)
	}
	report, err := reader.DescribeRepoSchema(scoped, head, "")
	if err != nil || len(report.Schemas) != 1 || len(report.Schemas[0].Fields) != 1 {
		t.Fatalf("the declared field's Schema must remain inside the Dataset: %#v %v", report, err)
	}
	plan, err := planDeclaredAccess(cat, "schema-only")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 || len(plan.Specs) != 1 {
		t.Fatalf("Dataset must select its Schema file without the index member: %#v", plan)
	}
	spec := plan.Specs[0]
	if len(spec.Fields) != 0 || len(spec.Declarations) != 0 || len(spec.QueryLanes()) != 0 {
		t.Fatalf("out-of-scope declaration witnesses must not grant retrieval capability: %#v", spec)
	}
}

func TestPlanAccessUnknownWorkspace(t *testing.T) {
	s := setupFed(t)
	_, err := planDeclaredAccess(s.catalog, "missing")
	testkit.ExpectCode(t, err, kernel.ErrDatasetInvalid)
}
