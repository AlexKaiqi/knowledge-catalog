package scenario

import (
	"testing"

	"kc/catalog"
	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

func TestCompanyWorkbench(t *testing.T) {
	wb := newWorkbench(t)
	steps := []struct {
		name string
		fn   func(*testing.T, *workbench)
	}{
		{"S0-empty-catalog", s0EmptyCatalog},
		{"S1-writes-leave-catalog", s1WritesLeaveCatalog},
		{"S2-define-workspace", s2DefineWorkspace},
		{"S3-claim-gmv", s3ClaimGMV},
		{"S4-personal-desk", s4PersonalDesk},
		{"S5-follow-published-branch", s5FollowPublishedBranch},
		{"S6-retire-and-archive", s6RetireAndArchive},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) { step.fn(t, wb) }) {
			return
		}
	}
}

func s0EmptyCatalog(t *testing.T, wb *workbench) {
	wb.stamp("owner", "s0-init", "")
	if err := wb.catalog.RecordCreated(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []kernel.RepositoryID{Metadata, Semantics, Personal} {
		if err := wb.catalog.RegisterRepository(id); err != nil {
			t.Fatal(err)
		}
	}

	_, err := wb.openView(ViewBoard)
	expectCode(t, err, kernel.ErrWorkspaceInvalid)

	_, err = wb.writer.CommitIntent("write-catalog-id", writer.CommitIntent{
		TargetRepository: kernel.RepositoryID(CatalogID),
		TargetRef:        MainRef,
		Operations:       []repository.Operation{putEntity("note/x", map[string]any{"v": "no"}, "")},
		Message:          "must not treat catalog as a knowledge repo",
	})
	expectCode(t, err, kernel.ErrTargetRepositoryDenied)

	_, err = wb.catalog.DefineWorkspace(ViewBoard, 1, []catalog.WorkspaceSource{
		{Repository: Unknown, Selector: MainRef},
	})
	expectCode(t, err, kernel.ErrWorkspaceInvalid)

	mustAllow(t, "collector", "commit", string(Metadata), "", "")
	mustDeny(t, "collector", "commit", string(Semantics), "", "")
	mustDeny(t, "collector", "propose", string(Semantics), "", "")
	mustAllow(t, "steward", "propose", string(Semantics), "", "")
	mustAllow(t, "steward", "define-workspace", "", CatalogID, "")
	mustAllow(t, "kai", "append", string(Personal), "", "")
	mustDeny(t, "kai", "commit", string(Semantics), "", "")
	mustAllow(t, "analyst-agent", "read-workspace", "", "", ViewBoard)
	mustDeny(t, "analyst-agent", "put", string(Semantics), "", "")
	mustDeny(t, "analyst-agent", "commit", string(Metadata), "", "")

	wb.expectCatalog(t, catalogWant{})
	if hist := wb.catalog.Log(catalog.CatalogLogQuery{Limit: 20}); len(hist.Commits) == 0 {
		t.Fatal("catalog init must leave registry git history")
	}
}

func s1WritesLeaveCatalog(t *testing.T, wb *workbench) {
	before := wb.freeze(t)

	_, err := wb.writer.CommitIntent("bad-schema", writer.CommitIntent{
		TargetRepository: Metadata,
		TargetRef:        MainRef,
		Operations: []repository.Operation{
			putAspect(TableTrade, "structure", map[string]any{"db": "dw"}, "schema/missing", ""),
		},
	})
	expectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
	wb.expectUnchanged(t, before)

	_, err = wb.writer.CommitIntent("bad-derivation", writer.CommitIntent{
		TargetRepository: Semantics,
		TargetRef:        MainRef,
		Operations:       []repository.Operation{putEntity("note/derived", map[string]any{"v": 1}, "")},
		Provenance:       &kernel.ProvenanceEnvelope{OriginKind: kernel.OriginDerivation},
	})
	expectCode(t, err, kernel.ErrPreconditionFailed)
	wb.expectUnchanged(t, before)

	_, err = wb.writer.AppendIntent("append-on-metadata", writer.AppendIntent{
		TargetRepository: Metadata,
		StreamRef:        StreamPractice,
		Entries: []repository.AppendEntry{{
			EventID: "evt-meta", Payload: map[string]any{"note": "streams are not snapshot members"},
		}},
	})
	expectCode(t, err, kernel.ErrTargetRepositoryDenied)
	wb.expectUnchanged(t, before)

	wb.stamp("collector", "s1-meta", "ingest")
	wb.mustCommit(t, "U1", "meta-u1", Metadata, metadataBoot(), sourceEnvelope("collector"))
	replay, err := wb.writer.CommitIntent("meta-u1", writer.CommitIntent{
		TargetRepository: Metadata,
		TargetRef:        MainRef,
		Operations:       metadataBoot(),
		Message:          "meta-u1",
		Provenance:       sourceEnvelope("collector"),
	})
	if err != nil || replay.Disposition != writer.DispositionReplayed || replay.Result.CommitID != wb.commits["U1"] {
		t.Fatalf("replay %#v %v", replay, err)
	}
	_, err = wb.writer.CommitIntent("meta-u1", writer.CommitIntent{
		TargetRepository: Metadata,
		TargetRef:        MainRef,
		Operations: []repository.Operation{
			putAspect(TableTrade, "structure", map[string]any{"db": "other"}, SchemaTableStruct, ""),
		},
	})
	expectCode(t, err, kernel.ErrIdempotencyConflict)
	if wb.head(t, Metadata) != wb.commits["U1"] {
		t.Fatal("conflict moved metadata HEAD")
	}

	wb.stamp("steward", "s1-sem", "seed")
	wb.mustCommit(t, "S1", "sem-s1", Semantics, semanticsBoot(), definitionEnvelope("steward"))
	mustResolve(t, wb.sem, MetricGMV, wb.commits["S1"], repository.StatusUnresolved)
	mustResolve(t, wb.sem, ExampleGMV, wb.commits["S1"], repository.StatusResolved)

	wb.stamp("kai", "s1-kai", "")
	wb.mustCommit(t, "K1", "kai-k1", Personal, personalBoot(), observationEnvelope("kai"))
	append1, err := wb.writer.AppendIntent("append-evt-1", writer.AppendIntent{
		TargetRepository: Personal,
		StreamRef:        StreamPractice,
		Entries: []repository.AppendEntry{{
			EventID: "evt-1", EventType: "review",
			Payload: map[string]any{"note": "退款口径疑问"},
		}},
	})
	if err != nil || append1.Disposition != writer.DispositionApplied {
		t.Fatal(append1, err)
	}
	replayed, err := wb.writer.AppendIntent("append-evt-1", writer.AppendIntent{
		TargetRepository: Personal,
		StreamRef:        StreamPractice,
		Entries: []repository.AppendEntry{{
			EventID: "evt-1", EventType: "review",
			Payload: map[string]any{"note": "退款口径疑问"},
		}},
	})
	if err != nil || replayed.Disposition != writer.DispositionReplayed {
		t.Fatal(replayed, err)
	}
	_, err = wb.writer.AppendIntent("append-evt-1-conflict", writer.AppendIntent{
		TargetRepository: Personal,
		StreamRef:        StreamPractice,
		Entries: []repository.AppendEntry{{
			EventID: "evt-1", Payload: map[string]any{"note": "different payload"},
		}},
	})
	expectCode(t, err, kernel.ErrEventIDConflict)

	page, err := wb.reader.ReadStream(Personal, StreamPractice)
	if err != nil || len(page.Records) != 1 || page.Records[0].EventID != "evt-1" {
		t.Fatalf("%#v %v", page, err)
	}

	if !wb.meta.HasCommit(wb.commits["U1"]) || wb.meta.HasCommit("not-a-commit") {
		t.Fatal("HasCommit")
	}
	table, err := wb.reader.Read(kernel.KnowledgeRef{Repository: Metadata, Object: TableTrade}, wb.commits["U1"], nil)
	if err != nil || nestedString(table.Value, "ownership", "owner") != "platform" {
		t.Fatalf("%#v %v", table, err)
	}

	wb.expectCatalog(t, catalogWant{})
}
