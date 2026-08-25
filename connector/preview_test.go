package connector_test

import (
	"testing"

	"kc/connector"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/writer"
)

func structure(objectID string) knowledge.Address {
	return knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.ObjectID(objectID), AspectName: "structure"}
}

func classification(objectID string) knowledge.Address {
	return knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.ObjectID(objectID), AspectName: "classification"}
}

func basePlan(mode connector.Mode, desired []connector.Unit, observed []connector.Observed) connector.Plan {
	return connector.Plan{
		ConnectorID:      "hive-structure",
		Mode:             mode,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: "kr://acme/public/physical",
		BaseCommit:       "P0",
		Desired:          desired,
		Observed:         observed,
		SourceRefs:       []string{"hive://c/db"},
		ProducedAt:       "2026-08-20T03:00:00Z",
	}
}

func TestPatchAddUpdateNoRemove(t *testing.T) {
	desired := []connector.Unit{
		{Address: structure("Table:c.db.a"), Value: map[string]any{"v": 1}},
		{Address: structure("Table:c.db.b"), Value: map[string]any{"v": 2}},
	}
	observed := []connector.Observed{
		{Address: structure("Table:c.db.b"), Digest: kernel.CanonicalDigest(map[string]any{"v": 0})},
		{Address: structure("Table:c.db.gone"), Digest: "stale"},
	}
	preview, err := connector.Preview(basePlan(connector.ModePatch, desired, observed))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Added != 1 || preview.Summary.Updated != 1 || preview.Summary.Removed != 0 {
		t.Fatalf("%#v", preview.Summary)
	}
	if len(preview.ChangeSet.Operations) != 2 {
		t.Fatalf("%#v", preview.ChangeSet.Operations)
	}
	if preview.ChangeSet.Provenance == nil || preview.ChangeSet.Provenance.OriginKind != knowledge.OriginSource {
		t.Fatal(preview.ChangeSet.Provenance)
	}
}

func TestReconcileRemovesAbsentFromSource(t *testing.T) {
	desired := []connector.Unit{
		{Address: structure("Table:c.db.keep"), Value: map[string]any{"v": 1}},
	}
	observed := []connector.Observed{
		{Address: structure("Table:c.db.keep"), Digest: kernel.CanonicalDigest(map[string]any{"v": 1})},
		{Address: structure("Table:c.db.gone"), Digest: "stale"},
	}
	preview, err := connector.Preview(basePlan(connector.ModeReconcile, desired, observed))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Added != 0 || preview.Summary.Updated != 0 || preview.Summary.Removed != 1 || preview.Summary.Unchanged != 1 {
		t.Fatalf("%#v", preview.Summary)
	}
	if preview.ChangeSet.Operations[0].Op != knowledge.OpRemove || preview.ChangeSet.Operations[0].Address.ObjectID != "Table:c.db.gone" {
		t.Fatalf("%#v", preview.ChangeSet.Operations)
	}
}

func TestScopeRejectsOtherAspect(t *testing.T) {
	plan := basePlan(connector.ModePatch, []connector.Unit{
		{Address: classification("Table:c.db.a"), Value: map[string]any{"pii": true}},
	}, nil)
	_, err := connector.Preview(plan)
	testkit.ExpectCode(t, err, kernel.ErrScopeDenied)
}

func TestReconcileIgnoresOutOfScopeObserved(t *testing.T) {
	preview, err := connector.Preview(basePlan(connector.ModeReconcile, nil, []connector.Observed{
		{Address: classification("Table:c.db.a"), Digest: "x"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Empty || preview.Summary.Removed != 0 || preview.Summary.Ignored != 1 {
		t.Fatalf("%#v", preview)
	}
}

func TestDuplicateDesired(t *testing.T) {
	addr := structure("Table:c.db.a")
	_, err := connector.Preview(basePlan(connector.ModePatch, []connector.Unit{
		{Address: addr, Value: map[string]any{"v": 1}},
		{Address: addr, Value: map[string]any{"v": 2}},
	}, nil))
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestUnchangedIsEmpty(t *testing.T) {
	value := map[string]any{"v": 1}
	preview, err := connector.Preview(basePlan(connector.ModePatch, []connector.Unit{
		{Address: structure("Table:c.db.a"), Value: value},
	}, []connector.Observed{
		{Address: structure("Table:c.db.a"), Digest: kernel.CanonicalDigest(value)},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Empty || preview.Summary.Unchanged != 1 || len(preview.ChangeSet.Operations) != 0 {
		t.Fatalf("%#v", preview)
	}
}

func TestRequiresSourceRefsAndScope(t *testing.T) {
	plan := basePlan(connector.ModePatch, nil, nil)
	plan.SourceRefs = nil
	_, err := connector.Preview(plan)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)

	plan = basePlan(connector.ModePatch, nil, nil)
	plan.Scope = connector.Scope{}
	_, err = connector.Preview(plan)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)

	plan = basePlan("full", nil, nil)
	_, err = connector.Preview(plan)
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestObjectPrefix(t *testing.T) {
	plan := basePlan(connector.ModePatch, []connector.Unit{
		{Address: structure("Column:c.db.t.col"), Value: map[string]any{"t": "int"}},
	}, nil)
	plan.Scope.ObjectPrefix = "Table:"
	_, err := connector.Preview(plan)
	testkit.ExpectCode(t, err, kernel.ErrScopeDenied)
}

func TestMemberInAspectScope(t *testing.T) {
	member := knowledge.Address{
		Kind:       knowledge.KindMember,
		ObjectID:   "ETLTask:job-1",
		AspectName: "io",
		MemberKey:  "in-1",
	}
	plan := basePlan(connector.ModePatch, []connector.Unit{
		{Address: member, Value: map[string]any{"urn": "t"}},
	}, nil)
	plan.Scope.Aspects = []string{"io"}
	preview, err := connector.Preview(plan)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Added != 1 || preview.ChangeSet.Operations[0].Address.MemberKey != "in-1" {
		t.Fatalf("%#v", preview)
	}
}

func TestCommandID(t *testing.T) {
	if got := connector.CommandID("hive-structure", "abc"); got != "connector:hive-structure:abc" {
		t.Fatal(got)
	}
	ops := []knowledge.Operation{{Op: knowledge.OpPut, Address: structure("Table:c.db.a")}}
	if connector.RunKey(ops) == "" || connector.RunKey(ops) != connector.RunKey(ops) {
		t.Fatal("run key must be stable")
	}
}

func TestPreviewThenCommit(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/public/physical")
	firstValue := map[string]any{"qualified_name": "db.t"}
	addr := structure("Table:c.db.t")
	first, err := s.Writer.Commit("boot", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:       knowledge.OpPut,
			Address:  addr,
			Value:    firstValue,
			PathHint: "table/t.json",
		}},
		Provenance: connector.Envelope("hive-structure", []string{"hive://c/db"}, "2026-08-20T02:00:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	next := map[string]any{"qualified_name": "db.t2"}
	preview, err := connector.Preview(connector.Plan{
		ConnectorID:      "hive-structure",
		Mode:             connector.ModePatch,
		Scope:            connector.Scope{Aspects: []string{"structure"}},
		TargetRepository: s.RepositoryID,
		BaseCommit:       first.Result.CommitID,
		Desired:          []connector.Unit{{Address: addr, Value: next, PathHint: "table/t.json"}},
		Observed:         []connector.Observed{{Address: addr, Digest: kernel.CanonicalDigest(firstValue)}},
		SourceRefs:       []string{"hive://c/db"},
		ProducedAt:       "2026-08-20T03:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Empty || preview.Summary.Updated != 1 {
		t.Fatalf("%#v", preview.Summary)
	}
	cmd := connector.CommandID("hive-structure", connector.RunKey(preview.ChangeSet.Operations))
	receipt, err := s.Writer.Commit(cmd, preview.ChangeSet)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != writer.DispositionApplied {
		t.Fatalf("%#v", receipt)
	}
	replay, err := s.Writer.Commit(cmd, preview.ChangeSet)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Disposition != writer.DispositionReplayed {
		t.Fatalf("%#v", replay)
	}
}

func TestAllowEntity(t *testing.T) {
	addr := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "note/1"}
	preview, err := connector.Preview(connector.Plan{
		ConnectorID:      "files",
		Mode:             connector.ModePatch,
		Scope:            connector.Scope{AllowEntity: true},
		TargetRepository: "kr://acme/public/core",
		Desired:          []connector.Unit{{Address: addr, Value: "hello"}},
		SourceRefs:       []string{"file://notes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Added != 1 {
		t.Fatalf("%#v", preview.Summary)
	}
}
