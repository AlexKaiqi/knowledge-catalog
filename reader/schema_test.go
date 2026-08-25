package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

func tableStructureSchema() map[string]any {
	return map[string]any{
		"entity":  "Table",
		"aspect":  "structure",
		"pattern": "record",
		"fields": map[string]any{
			"db":              map[string]any{"type": "string", "access": []any{"filter"}},
			"schema_name":     map[string]any{"type": "string", "access": []any{"filter"}},
			"raw_description": map[string]any{"type": "string", "access": []any{"text"}},
			"updated_at":      map[string]any{"type": "string", "access": []any{"sort"}},
		},
	}
}

func TestDescribeSchemaListsAccessHints(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: testkit.PutEntity("schema/dw.table.structure", tableStructureSchema(), ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Reader.DescribeSchema(s.RepositoryID, head, "")
	if err != nil || len(report.Schemas) != 1 {
		t.Fatalf("%#v %v", report, err)
	}
	got := report.Schemas[0]
	if got.Entity != "Table" || got.Aspect != "structure" || got.Pattern != "record" {
		t.Fatalf("%#v", got)
	}
	if len(got.Fields) != 4 || got.Fields[0].Path != "db" {
		t.Fatalf("%#v", got.Fields)
	}
	if !hasHints(got.Fields[0].Access, reader.HintFilter) {
		t.Fatalf("db hints %v", got.Fields[0].Access)
	}
}

func TestDescribeSchemaRejectsLegacyAndPhysicalAccessTokens(t *testing.T) {
	for _, token := range []string{"key", "summary", "stored", "gin", "hnsw"} {
		t.Run(token, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: "HEAD",
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: testkit.PutEntity("schema/bad", map[string]any{
					"entity": "Bad", "pattern": "record",
					"fields": map[string]any{"value": map[string]any{"access": []any{token}}},
				}, ""),
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Reader.DescribeSchema(s.RepositoryID, head, "")
			testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
		})
	}
}

func TestDescribeSchemaIgnoresNonSchemaObjects(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: append(
			testkit.PutEntity("schema/dw.table.structure", tableStructureSchema(), ""),
			testkit.PutEntity("Table:tl.db.t", map[string]any{"db": "tl"}, "")...,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Reader.DescribeSchema(s.RepositoryID, head, "")
	if err != nil || len(report.Schemas) != 1 || report.Schemas[0].ObjectID != "schema/dw.table.structure" {
		t.Fatalf("%#v %v", report, err)
	}
}

func TestDescribeSchemaFollowsSchemaRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{
			{
				Op:      repository.OpPut,
				Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/dw.table.structure"},
				Value:   tableStructureSchema(),
			},
			{
				Op:        repository.OpPut,
				Address:   kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:tl.db.t", AspectName: "structure"},
				SchemaRef: "schema/dw.table.structure",
				Value:     map[string]any{"db": "tl"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Reader.DescribeSchema(s.RepositoryID, head, "Table:tl.db.t")
	if err != nil || len(report.Schemas) != 1 || report.Schemas[0].ObjectID != "schema/dw.table.structure" {
		t.Fatalf("%#v %v", report, err)
	}
}

func TestDescribeSchemaUnresolvedRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []repository.Operation{{
			Op:        repository.OpPut,
			Address:   kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:tl.db.t", AspectName: "structure"},
			SchemaRef: "schema/missing",
			Value:     map[string]any{"db": "tl"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Reader.DescribeSchema(s.RepositoryID, head, "Table:tl.db.t")
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestDescribeSchemaOneObject(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: append(
			testkit.PutEntity("schema/dw.table.structure", tableStructureSchema(), ""),
			testkit.PutEntity("schema/dw.metric.definition", map[string]any{
				"entity": "Metric", "aspect": "definition", "pattern": "record",
				"fields": map[string]any{"expr": map[string]any{"access": []any{"text"}}},
			}, "")...,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Reader.DescribeSchema(s.RepositoryID, head, "schema/dw.metric.definition")
	if err != nil || len(report.Schemas) != 1 || report.Schemas[0].Entity != "Metric" {
		t.Fatalf("%#v %v", report, err)
	}
}

func hasHints(got []reader.AccessHint, want ...reader.AccessHint) bool {
	set := map[reader.AccessHint]struct{}{}
	for _, h := range got {
		set[h] = struct{}{}
	}
	for _, h := range want {
		if _, ok := set[h]; !ok {
			return false
		}
	}
	return true
}
