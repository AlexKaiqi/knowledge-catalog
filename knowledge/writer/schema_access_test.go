package writer_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestTemporalSchemaPublicationAndInstanceValidation(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		valid   string
		invalid []any
	}{
		{"date", "2024-02-29", []any{"2023-02-29", "2024-02-29T00:00:00Z", 20240229}},
		{"datetime", "2024-02-29T12:30:00.123456789000+08:00", []any{"2024-02-30T12:30:00Z", "2024-02-29T12:30:00", "2024-02-29T12:30:00.1234567891Z", 1709209800}},
		{"timestamp", "2024-02-29T04:30:00.000000001Z", []any{"2024-02-29", "2024-02-29T25:30:00Z", "2024-02-29T04:30:00.0000000001+08:00", true}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			schemaID := knowledge.ObjectID("schema/event/timing/" + tc.kind)
			address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "event/A", AspectName: "timing"}
			published, err := s.Writer.Commit("publish-temporal", knowledge.CommitChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: []knowledge.Operation{
					{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: schemaID}, Value: map[string]any{
						"entity": "Event", "aspect": "timing", "pattern": "record",
						"fields": map[string]any{"at": map[string]any{"type": tc.kind, "required": true, "access": []any{"filter", "sort"}}},
					}},
					{Op: knowledge.OpPut, Address: address, SchemaRef: string(schemaID), Value: map[string]any{"at": tc.valid}},
				},
			})
			if err != nil {
				t.Fatalf("publish valid %s schema and instance: %v", tc.kind, err)
			}
			commit := published.Result.CommitID
			report, err := s.Reader.DescribeSchema(s.RepositoryID, commit, schemaID)
			if err != nil {
				t.Fatal(err)
			}
			wantFields := []reader.FieldAccess{{Path: "at", Type: tc.kind, Access: []reader.AccessHint{reader.HintFilter, reader.HintSort}}}
			if len(report.Schemas) != 1 || report.Commit != commit || !reflect.DeepEqual(report.Schemas[0].Fields, wantFields) {
				t.Fatalf("published temporal access contract changed: %#v", report)
			}
			value, err := s.Reader.ReadAddress(s.RepositoryID, address, commit)
			if err != nil {
				t.Fatal(err)
			}
			if value.Commit != commit || value.Value.(map[string]any)["at"] != tc.valid {
				t.Fatalf("READ must retain Canonical time precision and zone: %#v", value)
			}
			for _, invalid := range tc.invalid {
				_, err := s.Writer.Commit("invalid-temporal", knowledge.CommitChangeSet{
					TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
					BaseCommit: commit, ExpectedTargetCommit: commit,
					Operations: []knowledge.Operation{{
						Op: knowledge.OpPut, Address: address, Value: map[string]any{"at": invalid},
					}},
				})
				testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
				if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != commit {
					t.Fatalf("invalid %s instance advanced HEAD to %s", tc.kind, got)
				}
			}
		})
	}
}

func TestSchemaPublicationRejectsUndefinedCompositeAccess(t *testing.T) {
	for _, kind := range []string{"object", "record", "array", "relation_endpoint_list"} {
		for _, access := range []string{"text", "filter", "sort"} {
			t.Run(kind+"/"+access, func(t *testing.T) {
				s := testkit.NewSetup(t, "")
				_, err := s.Writer.Commit("invalid-composite-access", knowledge.CommitChangeSet{
					TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
					BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
					Operations: []knowledge.Operation{{
						Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"},
						Value: map[string]any{"entity": "Policy", "fields": map[string]any{
							"value": map[string]any{"type": kind, "access": []any{access}},
						}},
					}},
				})
				testkit.ExpectCode(t, err, kernel.ErrSchemaUnsupported)
				if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
					t.Fatalf("undefined access advanced HEAD to %s", got)
				}
			})
		}
	}
}
