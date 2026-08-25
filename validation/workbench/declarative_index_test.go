package scenario

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kc/catalog"
	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/local"
	"kc/reader"
	"kc/repository"
	"kc/writer"
)

const declarativeIndexWorkspace = "declarative-index-five-entities"

type declarativeIndexHook struct {
	idx *index.Index
}

func (h declarativeIndexHook) AfterSnapshot(ev catalog.Snapshot) error {
	repo, ok := repository.KnowledgeOf(ev.Repository)
	if !ok {
		return nil
	}
	return h.idx.AfterSnapshot(repo, ev.From, ev.To, nil)
}

// TestDeclarativeIndexFiveWarehouseEntities is the data-warehouse acceptance
// matrix for AccessHints. It intentionally crosses the public layer seams:
// Writer -> Snapshot -> Catalog Hook -> Index -> Workspace pin -> Canonical read.
func TestDeclarativeIndexFiveWarehouseEntities(t *testing.T) {
	physicalID := kernel.RepositoryID("kr://acme/validation/physical")
	semanticID := kernel.RepositoryID("kr://acme/validation/semantic")
	store := repository.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	physical := testkit.MakeRepository(t, string(physicalID))
	semantic := testkit.MakeRepository(t, string(semanticID))
	for _, repo := range []repository.Repository{physical, semantic} {
		if err := store.Add(repo); err != nil {
			t.Fatal(err)
		}
	}

	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := catalog.NewRegistry(testkit.TempDir(t), "kr://acme/validation/catalog")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewCatalog(store, registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []kernel.RepositoryID{physicalID, semanticID} {
		if err := cat.RegisterRepository(id); err != nil {
			t.Fatal(err)
		}
	}

	idx := index.NewIndexEngine(testkit.TempDir(t), local.OpenSQLite)
	t.Cleanup(func() { _ = idx.Close() })
	cat.AddHook(declarativeIndexHook{idx: idx})

	physicalV1 := declarativeCommit(t, w, "physical-five-entities-v1", physicalID, []repository.Operation{
		declarativeSchema("schema/table.structure", "Table", "structure", map[string]any{
			"kind":        declarativeField("string", "filter"),
			"name":        declarativeField("string", "filter", "text"),
			"description": declarativeField("string", "text"),
			"database":    declarativeField("string", "filter"),
		}),
		declarativeSchema("schema/column.structure", "Column", "structure", map[string]any{
			"kind":        declarativeField("string", "filter"),
			"name":        declarativeField("string", "filter", "text"),
			"table_id":    declarativeField("string", "filter"),
			"data_type":   declarativeField("string", "filter"),
			"ordinal":     declarativeField("number", "filter", "sort"),
			"description": declarativeField("string", "text"),
		}),
		declarativeAspect("Table:dw.orders", "structure", "schema/table.structure", map[string]any{
			"kind": "table", "name": "orders", "description": "commerce order facts", "database": "dw",
			"canonical_note": "returned from canonical, not the index",
		}),
		declarativeAspect("Column:dw.orders.order_id", "structure", "schema/column.structure", map[string]any{
			"kind": "column", "name": "order_id", "table_id": "Table:dw.orders", "data_type": "bigint",
			"ordinal": 7, "description": "order identifier",
		}),
	})

	semanticV1 := declarativeCommit(t, w, "semantic-five-entities-v1", semanticID, []repository.Operation{
		declarativeSchema("schema/metric.definition", "Metric", "definition", map[string]any{
			"kind":        declarativeField("string", "filter"),
			"name":        declarativeField("string", "filter", "text"),
			"description": declarativeField("string", "text"),
			"formula":     declarativeField("string", "text"),
			"domain":      declarativeField("string", "filter"),
		}),
		declarativeSchema("schema/dimension.definition", "Dimension", "definition", map[string]any{
			"kind":        declarativeField("string", "filter"),
			"name":        declarativeField("string", "filter", "text"),
			"description": declarativeField("string", "text"),
			"role":        declarativeField("string", "filter"),
			"data_type":   declarativeField("string", "filter"),
		}),
		declarativeSchema("schema/measure.definition", "Measure", "definition", map[string]any{
			"kind":        declarativeField("string", "filter"),
			"name":        declarativeField("string", "filter", "text"),
			"description": declarativeField("string", "text"),
			"expression":  declarativeField("string", "text"),
			"aggregation": declarativeField("string", "filter"),
			"updated_at":  declarativeField("datetime", "sort"),
		}),
		declarativeAspect("Metric:gmv", "definition", "schema/metric.definition", map[string]any{
			"kind": "metric", "name": "gmv", "description": "transactionvolume baseline",
			"formula": "sum(pay_amount)", "domain": "trade",
		}),
		declarativeAspect("Dimension:order_date", "definition", "schema/dimension.definition", map[string]any{
			"kind": "dimension", "name": "order_date", "description": "business calendar date",
			"role": "time", "data_type": "date",
		}),
		declarativeAspect("Measure:net_revenue", "definition", "schema/measure.definition", map[string]any{
			"kind": "measure", "name": "net_revenue", "description": "net revenue amount",
			"expression": "pay_amount-refund_amount", "aggregation": "sum", "unit": "currency",
			"updated_at": "2026-08-23T10:00:00+08:00",
		}),
		declarativeAspect("Measure:pay_amount", "definition", "schema/measure.definition", map[string]any{
			"kind": "measure", "name": "pay_amount", "description": "paid amount",
			"expression": "pay_amount", "aggregation": "sum", "unit": "currency",
			"updated_at": "2026-08-24T10:00:00+08:00",
		}),
	})

	if _, err := cat.DefineWorkspace(declarativeIndexWorkspace, 1, []catalog.WorkspaceSource{
		{Repository: physicalID, Selector: MainRef},
		{Repository: semanticID, Selector: MainRef},
	}); err != nil {
		t.Fatal(err)
	}
	planV1 := declarativePlan(t, cat)
	if len(planV1.Specs) != 2 {
		t.Fatalf("want one access spec per repository, got %#v", planV1.Specs)
	}
	if got := declarativeSchemaCount(planV1); got != 5 {
		t.Fatalf("want five compiled warehouse schemas, got %d: %#v", got, planV1)
	}

	entityHits := map[string]string{}
	assertOne := func(name, want string, req reader.SearchRequest) repository.KnowledgeValue {
		t.Helper()
		hits := declarativeSearch(t, idx, store, planV1, req)
		if len(hits) != 1 || string(hits[0].Address.ObjectID) != want {
			t.Fatalf("%s: want %s, got %#v", name, want, declarativeObjectIDs(hits))
		}
		entityHits[name] = want
		return hits[0]
	}

	tableHit := assertOne("table", "Table:dw.orders", reader.SearchOf(
		declarativeEQ("schema/table.structure", "structure", "kind", "table"),
		declarativeMatch("schema/table.structure", "structure", "name", "orders"),
	))
	if nestedString(tableHit.Value, "structure", "canonical_note") != "returned from canonical, not the index" {
		t.Fatalf("search hit did not hydrate canonical content: %#v", tableHit.Value)
	}
	assertOne("column", "Column:dw.orders.order_id", reader.SearchOf(
		declarativeEQ("schema/column.structure", "structure", "kind", "column"),
		declarativeEQ("schema/column.structure", "structure", "data_type", "bigint"),
	))
	assertOne("column-range", "Column:dw.orders.order_id", reader.SearchOf(
		declarativeEQ("schema/column.structure", "structure", "kind", "column"),
		declarativeRange(reader.OpGT, "schema/column.structure", "structure", "ordinal", "5"),
	))
	assertOne("metric", "Metric:gmv", reader.SearchOf(
		declarativeEQ("schema/metric.definition", "definition", "kind", "metric"),
		declarativeMatch("schema/metric.definition", "definition", "description", "transactionvolume"),
	))
	assertOne("dimension", "Dimension:order_date", reader.SearchOf(
		declarativeEQ("schema/dimension.definition", "definition", "kind", "dimension"),
		declarativeEQ("schema/dimension.definition", "definition", "role", "time"),
	))
	assertOne("measure", "Measure:net_revenue", reader.SearchOf(
		declarativeEQ("schema/measure.definition", "definition", "kind", "measure"),
		declarativeMatch("schema/measure.definition", "definition", "name", "net_revenue"),
	))

	sortedMeasures := declarativeSearch(t, idx, store, planV1, reader.SearchOf(
		declarativeEQ("schema/measure.definition", "definition", "kind", "measure"),
		declarativeSort("schema/measure.definition", "definition", "updated_at", "asc"),
	))
	if got := declarativeObjectIDs(sortedMeasures); len(got) != 2 || got[0] != "Measure:net_revenue" || got[1] != "Measure:pay_amount" {
		t.Fatalf("measure sort: %#v", got)
	}

	beforeUnit := declarativeSearchError(idx, store, planV1, reader.SearchOf(
		declarativeEQ("schema/measure.definition", "definition", "unit", "currency"),
	))
	if kernel.CodeOf(beforeUnit) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("unit must be unavailable before it is declared: %v", beforeUnit)
	}

	semanticV2 := declarativeCommit(t, w, "metric-content-v2", semanticID, []repository.Operation{
		declarativeAspect("Metric:gmv", "definition", "schema/metric.definition", map[string]any{
			"kind": "metric", "name": "gmv", "description": "revenuevelocity governed",
			"formula": "sum(pay_amount)", "domain": "trade",
		}),
	})
	contentDesc, err := idx.Describe(semantic)
	if err != nil {
		t.Fatal(err)
	}
	if contentDesc.BasisCommit != semanticV2 || contentDesc.Mode != index.IndexModeIncremental || contentDesc.Cause != index.IndexCauseContent {
		t.Fatalf("content update must be incremental: %#v", contentDesc)
	}
	planV2 := declarativePlan(t, cat)
	newMetric := declarativeSearch(t, idx, store, planV2, reader.SearchOf(
		declarativeEQ("schema/metric.definition", "definition", "kind", "metric"),
		declarativeMatch("schema/metric.definition", "definition", "description", "revenuevelocity"),
	))
	if len(newMetric) != 1 || newMetric[0].Address.ObjectID != "Metric:gmv" {
		t.Fatalf("incremental metric search: %#v", declarativeObjectIDs(newMetric))
	}

	semanticV3 := declarativeCommit(t, w, "measure-schema-v2", semanticID, []repository.Operation{
		declarativeSchema("schema/measure.definition", "Measure", "definition", map[string]any{
			"kind":        declarativeField("string", "filter"),
			"name":        declarativeField("string", "filter", "text"),
			"description": declarativeField("string", "text"),
			"expression":  declarativeField("string", "text"),
			"aggregation": declarativeField("string", "filter"),
			"unit":        declarativeField("string", "filter"),
			"updated_at":  declarativeField("datetime", "sort"),
		}),
	})
	rebuildDesc, err := idx.Describe(semantic)
	if err != nil {
		t.Fatal(err)
	}
	if rebuildDesc.BasisCommit != semanticV3 || rebuildDesc.Mode != index.IndexModeRebuild || rebuildDesc.Cause != index.IndexCauseSchema {
		t.Fatalf("AccessHints update must rebuild: %#v", rebuildDesc)
	}
	planV3 := declarativePlan(t, cat)
	unitHits := declarativeSearch(t, idx, store, planV3, reader.SearchOf(
		declarativeEQ("schema/measure.definition", "definition", "kind", "measure"),
		declarativeEQ("schema/measure.definition", "definition", "unit", "currency"),
	))
	if len(unitHits) != 2 {
		t.Fatalf("newly declared unit filter: %#v", declarativeObjectIDs(unitHits))
	}

	oldMetric := declarativeSearch(t, idx, store, planV1, reader.SearchOf(
		declarativeEQ("schema/metric.definition", "definition", "kind", "metric"),
		declarativeMatch("schema/metric.definition", "definition", "description", "transactionvolume"),
	))
	if len(oldMetric) != 1 || oldMetric[0].Commit != semanticV1 {
		t.Fatalf("old Workspace pin must retain old metric: %#v", oldMetric)
	}
	oldCannotSeeNew := declarativeSearch(t, idx, store, planV1, reader.SearchOf(
		declarativeEQ("schema/metric.definition", "definition", "kind", "metric"),
		declarativeMatch("schema/metric.definition", "definition", "description", "revenuevelocity"),
	))
	if len(oldCannotSeeNew) != 0 {
		t.Fatalf("old Workspace pin saw new content: %#v", declarativeObjectIDs(oldCannotSeeNew))
	}
	liveAfterOldPin, err := idx.Describe(semantic)
	if err != nil {
		t.Fatal(err)
	}
	if liveAfterOldPin.BasisCommit != semanticV3 {
		t.Fatalf("old pin search rewound live projection: %#v", liveAfterOldPin)
	}

	evidence := map[string]any{
		"validation":        "declarative-index-five-warehouse-entities",
		"outcome":           "PASSED",
		"executedAt":        time.Now().Format(time.RFC3339),
		"workspace":         declarativeIndexWorkspace,
		"repositories":      []kernel.RepositoryID{physicalID, semanticID},
		"entityHits":        entityHits,
		"sortedMeasures":    declarativeObjectIDs(sortedMeasures),
		"physicalCommit":    physicalV1,
		"semanticV1":        semanticV1,
		"semanticV2":        semanticV2,
		"semanticV3":        semanticV3,
		"contentSyncMode":   contentDesc.Mode,
		"contentSyncCause":  contentDesc.Cause,
		"schemaSyncMode":    rebuildDesc.Mode,
		"schemaSyncCause":   rebuildDesc.Cause,
		"oldPinStable":      true,
		"canonicalHydrated": true,
		"assertions": []string{
			"five schema objects compiled across two repository projections",
			"table column metric dimension measure queries hit expected canonical objects",
			"range and sort operators follow AccessHints",
			"undeclared unit filter fails with CAPABILITY_UNSATISFIED",
			"content change updates incrementally",
			"AccessHints change rebuilds and enables the new unit filter",
			"old Workspace pin remains reproducible and does not rewind live index",
		},
	}
	writeDeclarativeEvidence(t, evidence)
}

func declarativeField(fieldType string, access ...string) map[string]any {
	faces := make([]any, len(access))
	for i, face := range access {
		faces[i] = face
	}
	return map[string]any{"type": fieldType, "access": faces}
}

func declarativeSchema(objectID, entity, aspect string, fields map[string]any) repository.Operation {
	return repository.Operation{
		Op:      repository.OpPut,
		Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: kernel.ObjectID(objectID)},
		Value: map[string]any{
			"entity": entity, "aspect": aspect, "pattern": "record", "fields": fields,
		},
	}
}

func declarativeAspect(objectID, aspect, schemaRef string, value map[string]any) repository.Operation {
	return repository.Operation{
		Op:        repository.OpPut,
		Address:   kernel.Address{Kind: kernel.KindAspect, ObjectID: kernel.ObjectID(objectID), AspectName: aspect},
		SchemaRef: schemaRef,
		Value:     value,
	}
}

func declarativeCommit(t *testing.T, w *writer.Writer, commandID string, repo kernel.RepositoryID, ops []repository.Operation) kernel.CommitID {
	t.Helper()
	receipt, err := w.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository: repo,
		TargetRef:        MainRef,
		Operations:       ops,
		Message:          commandID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != writer.DispositionApplied {
		t.Fatalf("%s disposition %s", commandID, receipt.Disposition)
	}
	return receipt.Result.CommitID
}

func declarativePlan(t *testing.T, cat *catalog.Catalog) reader.AccessPlan {
	t.Helper()
	resolved, err := cat.ResolveWorkspace(declarativeIndexWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := reader.PlanAccess(cat.RequireKnowledge, workspacePin(resolved))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func declarativeSchemaCount(plan reader.AccessPlan) int {
	total := 0
	for _, spec := range plan.Specs {
		total += len(spec.Schemas)
	}
	return total
}

func declarativeFieldRef(schema, aspect, path string) *reader.FieldRef {
	return &reader.FieldRef{Schema: kernel.ObjectID(schema), Aspect: aspect, Path: path}
}

func declarativeEQ(schema, aspect, path, value string) reader.SearchClause {
	return reader.SearchClause{Op: reader.OpEQ, Field: declarativeFieldRef(schema, aspect, path), Value: value}
}

func declarativeMatch(schema, aspect, path, value string) reader.SearchClause {
	return reader.SearchClause{Op: reader.OpMatch, Field: declarativeFieldRef(schema, aspect, path), Value: value}
}

func declarativeRange(op reader.SearchOp, schema, aspect, path, value string) reader.SearchClause {
	return reader.SearchClause{Op: op, Field: declarativeFieldRef(schema, aspect, path), Value: value}
}

func declarativeSort(schema, aspect, path, order string) reader.SearchClause {
	return reader.SearchClause{Op: reader.OpSort, Field: declarativeFieldRef(schema, aspect, path), Order: order}
}

func declarativeSearch(t *testing.T, idx *index.Index, store *repository.Store, plan reader.AccessPlan, req reader.SearchRequest) []repository.KnowledgeValue {
	t.Helper()
	hits, err := declarativeSearchResult(idx, store, plan, req)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func declarativeSearchError(idx *index.Index, store *repository.Store, plan reader.AccessPlan, req reader.SearchRequest) error {
	_, err := declarativeSearchResult(idx, store, plan, req)
	return err
}

func declarativeSearchResult(idx *index.Index, store *repository.Store, plan reader.AccessPlan, req reader.SearchRequest) ([]repository.KnowledgeValue, error) {
	var out []repository.KnowledgeValue
	tried, unsatisfied := 0, 0
	for _, spec := range plan.Specs {
		repo, err := store.Knowledge(spec.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return nil, err
		}
		tried++
		result, err := idx.SearchAt(repo, spec.Commit, req)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
				unsatisfied++
				continue
			}
			return nil, err
		}
		for _, hit := range result.Hits {
			out = append(out, hit.Knowledge)
		}
	}
	if tried > 0 && tried == unsatisfied {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "no member index satisfies this search")
	}
	return out, nil
}

func declarativeObjectIDs(hits []repository.KnowledgeValue) []string {
	ids := make([]string, len(hits))
	for i, hit := range hits {
		ids[i] = string(hit.Address.ObjectID)
	}
	return ids
}

func writeDeclarativeEvidence(t *testing.T, evidence map[string]any) {
	t.Helper()
	path := os.Getenv("KC_DECLARATIVE_INDEX_REPORT")
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("validation evidence: %s", path)
}
