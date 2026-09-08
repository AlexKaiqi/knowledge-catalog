package index

import (
	"reflect"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
	"kc/snapshot"
)

func TestProjectionCompilerOnlyBuildsDeclaredSlots(t *testing.T) {
	for _, hints := range [][]reader.AccessHint{
		{reader.HintText}, {reader.HintFilter}, {reader.HintSort},
		{reader.HintText, reader.HintFilter}, {reader.HintText, reader.HintSort},
	} {
		field := retrieval.AccessField{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: hints}
		body := strings.Repeat("规范知识与中文正文 ", 5000)
		cell, err := projectionCell(field, body)
		if err != nil {
			t.Fatal(err)
		}
		wantScalar := field.Has(reader.HintFilter) || field.Has(reader.HintSort)
		if (cell.StringValue != nil) != wantScalar {
			t.Fatalf("access %v generated an undeclared scalar slot: present=%v", hints, cell.StringValue != nil)
		}
		if field.Has(reader.HintText) != (cell.TextValue == body) || cell.Value != body {
			t.Fatalf("access %v lost its declared text or canonical value", hints)
		}
	}
}

func TestProjectionDigestIncludesSearchableTextOrder(t *testing.T) {
	field := retrieval.AccessField{FieldRef: retrieval.FieldRef{Schema: "schema/tags", Aspect: "tags", Path: "label"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}
	value := knowledge.KnowledgeValue{
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "article/one"},
		Value:   map[string]any{"tags": map[string]any{"a": map[string]any{"label": "alpha"}, "b": map[string]any{"label": "beta"}}},
		Declarations: []knowledge.UnitDeclaration{
			{Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "article/one", AspectName: "tags", MemberKey: "a"}, SchemaRef: "schema/tags"},
			{Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "article/one", AspectName: "tags", MemberKey: "b"}, SchemaRef: "schema/tags"},
		},
	}
	first, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	value.Declarations[0], value.Declarations[1] = value.Declarations[1], value.Declarations[0]
	second, err := compileProjectionDocument(nil, value, retrieval.AccessSpec{Fields: []retrieval.AccessField{field}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Text != second.Text && first.ObjectDigest == second.ObjectDigest {
		t.Fatal("different searchable text must not be mistaken for an unchanged projection")
	}
}

type projectionDeltaEngine struct {
	stateTestEngine
	upserts int
	deletes int
}

func (e *projectionDeltaEngine) Apply(upserts []CompiledDoc, deletes []knowledge.ObjectID, meta Meta) error {
	e.upserts, e.deletes = len(upserts), len(deletes)
	return e.stateTestEngine.Apply(upserts, deletes, meta)
}

type projectionBatchRepository struct {
	*testkit.KnowledgeRepository
	reads   int
	batches []int
}

func (r *projectionBatchRepository) Read(id knowledge.ObjectID, commit kernel.CommitID) (knowledge.KnowledgeValue, error) {
	r.reads++
	return r.KnowledgeRepository.Read(id, commit)
}

func (r *projectionBatchRepository) ReadMany(ids []knowledge.ObjectID, commit kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	r.batches = append(r.batches, len(ids))
	return r.KnowledgeRepository.ReadMany(ids, commit)
}

func TestIncrementalProjectionSkipsUnchangedDocumentsAndMatchesRebuild(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://projection/reliability")
	commit := func(base kernel.CommitID, ops ...knowledge.Operation) kernel.CommitID {
		t.Helper()
		result, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	put := func(id, body, note string) knowledge.Operation {
		return knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID(id)}, SchemaRef: "schema/article", Value: map[string]any{"body": body, "note": note}}
	}
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	first := commit(root,
		knowledge.Operation{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}}}},
		put("article/a", "alpha", "first"), put("article/b", "beta", "first"), put("article/c", "gamma", "first"),
	)
	engine := &projectionDeltaEngine{}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(repo, first); err != nil {
		t.Fatal(err)
	}
	second := commit(first, put("article/a", "alpha", "changed only unindexed content"))
	result, err := idx.Apply(repo, first, second, []knowledge.ObjectID{"article/a", "article/a"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 0 || engine.upserts != 0 || engine.deletes != 0 || engine.meta.Basis != second {
		t.Fatalf("unchanged projection must only advance basis: sync=%+v upserts=%d deletes=%d meta=%+v", result, engine.upserts, engine.deletes, engine.meta)
	}
	third := commit(second, put("article/a", "delta", "second"), knowledge.Operation{Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "article/b"}})
	spec, err := specAtCommit(repo, third)
	if err != nil {
		t.Fatal(err)
	}
	counted := &projectionBatchRepository{KnowledgeRepository: repo}
	result, err = idx.apply(engine, counted, second, third, spec, []knowledge.ObjectID{"article/a", "article/b", "article/a"}, IndexCauseContent)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Removed != 1 || engine.upserts != 1 || engine.deletes != 1 {
		t.Fatalf("only actual projection changes should be written: %+v", result)
	}
	if counted.reads != 0 || !reflect.DeepEqual(counted.batches, []int{2, 2}) {
		t.Fatalf("compare only changed identities in same-basis batches: reads=%d batches=%v", counted.reads, counted.batches)
	}
	rebuilt := &stateTestEngine{}
	if _, err := idx.rebuild(rebuilt, repo, third, spec, IndexCauseContent); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(engine.docs, rebuilt.docs) {
		t.Fatalf("incremental documents differ from rebuild: incremental=%v rebuild=%v", engine.docs, rebuilt.docs)
	}
}
