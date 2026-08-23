package scenario

import (
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

func s4PersonalDesk(t *testing.T, wb *workbench) {
	wb.stamp("kai", "s4-desk", "")
	if _, err := wb.catalog.DefineWorkspace(ViewDesk, 1, []catalog.WorkspaceSource{
		{Repository: Personal, Selector: MainRef},
	}); err != nil {
		t.Fatal(err)
	}
	wb.expectCatalog(t, s4Want(1, []kernel.RepositoryID{Metadata, Semantics}))

	habits, err := wb.federatedRead(ViewDesk, HabitMorning)
	if err != nil || len(habits) != 1 {
		t.Fatalf("%#v %v", habits, err)
	}
	deskGMV, err := wb.federatedRead(ViewDesk, MetricGMV)
	if err != nil || len(deskGMV) != 0 {
		t.Fatalf("desk has no GMV at K1: %#v", deskGMV)
	}

	wb.mustCommit(t, "K2", "kai-gmv-draft", Personal, gmvDraft(GMVPersonal), observationEnvelope("kai"))
	deskGMV, err = wb.federatedRead(ViewDesk, MetricGMV)
	if err != nil || len(deskGMV) != 1 || nestedString(deskGMV[0].Value, "definition", "formula") != GMVPersonal {
		t.Fatalf("desk follows personal main: %#v %v", deskGMV, err)
	}
	stable, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || nestedString(stable[0].Value, "definition", "formula") != GMVCompany {
		t.Fatalf("company board %#v %v", stable, err)
	}

	wb.stamp("steward", "s4-overlay", "")
	if _, err := wb.catalog.DefineWorkspace(ViewBoard, 2, wb.overlaySources()); err != nil {
		t.Fatal(err)
	}
	hits, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || len(hits) != 2 {
		t.Fatalf("federation must not overlay: %#v %v", hits, err)
	}
	company, ok := findFederated(hits, Semantics)
	if !ok || nestedString(company.Value, "definition", "formula") != GMVCompany {
		t.Fatalf("company value missing: %#v", hits)
	}
	draft, ok := findFederated(hits, Personal)
	if !ok || nestedString(draft.Value, "definition", "formula") != GMVPersonal {
		t.Fatalf("personal value missing: %#v", hits)
	}
	if _, ok := findFederated(hits, Metadata); ok {
		t.Fatal("metadata must not invent Metric:gmv")
	}

	if _, err := wb.catalog.DefineWorkspace(ViewBoard, 3, companyWorkspaceSources()); err != nil {
		t.Fatal(err)
	}
	board, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || len(board) != 1 || board[0].Repository != Semantics {
		t.Fatalf("recipe change must take effect on next read: %#v %v", board, err)
	}
	wb.expectCatalog(t, s4Want(3, []kernel.RepositoryID{Metadata, Semantics}))
}

func s4Want(boardRev int, boardRepos []kernel.RepositoryID) catalogWant {
	return catalogWant{
		workspaces: []workspaceWant{
			{id: ViewBoard, rev: boardRev, repos: boardRepos},
			{id: ViewDesk, rev: 1, repos: []kernel.RepositoryID{Personal}},
		},
	}
}

func s5FollowPublishedBranch(t *testing.T, wb *workbench) {
	wb.stamp("steward", "s5-move", "")
	old := mustResolve(t, wb.sem, ExampleGMV, wb.commits["C1"], repository.StatusResolved)
	wb.mustCommit(t, "S2", "sem-path-example", Semantics, []repository.Operation{
		putAspect(ExampleGMV, "body", map[string]any{"prompt": "退货是否算进 GMV？"}, SchemaExampleBody, "semantics/examples/gmv-refund.md"),
	}, definitionEnvelope("steward"))
	moved := mustResolve(t, wb.sem, ExampleGMV, wb.commits["S2"], repository.StatusResolved)
	if moved.PathHint != "semantics/examples/gmv-refund.md" || moved.ObjectID != ExampleGMV {
		t.Fatalf("path move %#v", moved)
	}
	pinned := mustResolve(t, wb.sem, ExampleGMV, wb.commits["C1"], repository.StatusResolved)
	if pinned.PathHint != old.PathHint {
		t.Fatalf("old commit must keep old path: %#v", pinned)
	}
	serving, err := wb.openView(ViewBoard)
	if err != nil {
		t.Fatal(err)
	}
	liveExample, err := serving.Resolve(ExampleGMV)
	if err != nil || len(liveExample) != 1 || liveExample[0].PathHint != moved.PathHint {
		t.Fatalf("next OpenWorkspace must follow published branch: %#v %v", liveExample, err)
	}

	wb.stamp("collector", "s5-owner", "ingest")
	wb.mustCommit(t, "U2", "meta-owner", Metadata, []repository.Operation{
		putAspect(TableTrade, "ownership", map[string]any{"owner": "platform-ops"}, SchemaTableOwner, "tables/dwd.trade_order.ownership.json"),
	}, sourceEnvelope("collector"))

	frozenHead := wb.head(t, Metadata)
	_, err = wb.writer.CommitIntent("meta-if-absent", writer.CommitIntent{
		TargetRepository: Metadata,
		TargetRef:        MainRef,
		Operations: []repository.Operation{{
			Op:           repository.OpPut,
			Address:      kernel.Address{Kind: kernel.KindAspect, ObjectID: TableTrade, AspectName: "structure"},
			Value:        map[string]any{"db": "dw", "name": "dwd.trade_order"},
			SchemaRef:    SchemaTableStruct,
			Precondition: &repository.Precondition{Type: repository.IfAbsent},
		}},
	})
	expectCode(t, err, kernel.ErrPreconditionFailed)
	if wb.head(t, Metadata) != frozenHead {
		t.Fatal("IfAbsent moved HEAD")
	}

	_, err = wb.writer.Commit("stale-cas", repository.CommitChangeSet{
		TargetRepository:     Metadata,
		TargetRef:            MainRef,
		BaseCommit:           wb.commits["U1"],
		ExpectedTargetCommit: wb.commits["U1"],
		Operations: []repository.Operation{
			putAspect(TableTrade, "ownership", map[string]any{"owner": "stale"}, SchemaTableOwner, ""),
		},
		Provenance: sourceEnvelope("collector"),
	})
	expectCode(t, err, kernel.ErrNonFastForward)

	trace, err := wb.reader.GetProvenance(kernel.KnowledgeRef{Repository: Semantics, Object: MetricGMV}, wb.commits["C1"])
	if err != nil || len(trace.Chain) == 0 || trace.Chain[0].OriginKind != kernel.OriginDefinition {
		t.Fatalf("%#v %v", trace, err)
	}
	addr, err := wb.reader.ReadAddress(Semantics, kernel.Address{
		Kind: kernel.KindAspect, ObjectID: MetricGMV, AspectName: "definition",
	}, wb.commits["C1"])
	if err != nil || nestedString(addr.Value, "formula") != GMVCompany {
		t.Fatalf("ReadAddress %#v %v", addr, err)
	}
	revs, err := wb.reader.Log(Semantics, ExampleGMV, wb.commits["S2"], 10)
	if err != nil || len(revs) < 1 {
		t.Fatalf("log %#v %v", revs, err)
	}
	diff, err := wb.reader.Diff(Metadata, TableTrade, wb.commits["U1"], wb.commits["U2"])
	if err != nil || diff.To == nil || nestedString(diff.To.Value, "ownership", "owner") != "platform-ops" {
		t.Fatalf("diff %#v %v", diff, err)
	}
	hits, err := wb.reader.Search("退款", Personal, wb.commits["K1"])
	if err != nil {
		t.Fatal(err)
	}
	foundDist := false
	for _, hit := range hits {
		if hit.Address.ObjectID == DistErrors {
			foundDist = true
		}
	}
	if !foundDist {
		t.Fatalf("search missed Dist: %#v", hits)
	}

	plan, err := wb.planIndex(ViewBoard)
	if err != nil || plan.WorkspaceID != ViewBoard || len(plan.Projections) != 2 {
		t.Fatalf("%#v %v", plan, err)
	}
	listed, err := serving.List()
	if err != nil || len(listed) == 0 {
		t.Fatal(listed, err)
	}
	schemas, err := serving.DescribeSchema("")
	if err != nil || len(schemas) == 0 {
		t.Fatal(schemas, err)
	}

	live, err := wb.federatedRead(ViewBoard, MetricGMV)
	if err != nil || nestedString(live[0].Value, "definition", "formula") != GMVCompany {
		t.Fatalf("%#v %v", live, err)
	}
	wb.expectCatalog(t, s4Want(3, []kernel.RepositoryID{Metadata, Semantics}))
}

func s6RetireAndArchive(t *testing.T, wb *workbench) {
	wb.stamp("steward", "s6-retire", "")
	if err := wb.catalog.RetireWorkspace(ViewBoard); err != nil {
		t.Fatal(err)
	}
	_, err := wb.openView(ViewBoard)
	expectCode(t, err, kernel.ErrWorkspaceInvalid)
	desk, err := wb.federatedRead(ViewDesk, HabitMorning)
	if err != nil || len(desk) != 1 {
		t.Fatalf("retiring the board must leave kai-desk: %#v %v", desk, err)
	}

	if err := wb.meta.Archive(); err != nil {
		t.Fatal(err)
	}
	_, err = wb.writer.CommitIntent("meta-after-archive", writer.CommitIntent{
		TargetRepository: Metadata,
		TargetRef:        MainRef,
		Operations: []repository.Operation{
			putAspect(TableTrade, "ownership", map[string]any{"owner": "gone"}, SchemaTableOwner, ""),
		},
		Provenance: sourceEnvelope("collector"),
	})
	expectCode(t, err, kernel.ErrRepositoryArchived)

	if err := wb.catalog.Archive(); err != nil {
		t.Fatal(err)
	}
	_, err = wb.catalog.DefineWorkspace("late", 1, []catalog.WorkspaceSource{{Repository: Personal, Selector: MainRef}})
	expectCode(t, err, kernel.ErrCatalogArchived)

	wb.stamp("kai", "s6-personal", "")
	wb.mustCommit(t, "K3", "kai-after-archive-catalog", Personal, []repository.Operation{
		putAspect(HabitMorning, "note", map[string]any{"when": "morning", "text": "catalog 归档不影响个人仓"}, SchemaHabitNote, ""),
	}, observationEnvelope("kai"))

	wb.expectCatalog(t, catalogWant{
		workspaces: []workspaceWant{
			{id: ViewBoard, rev: 3, retired: true, repos: []kernel.RepositoryID{Metadata, Semantics}},
			{id: ViewDesk, rev: 1, repos: []kernel.RepositoryID{Personal}},
		},
		archived: true,
	})
}
