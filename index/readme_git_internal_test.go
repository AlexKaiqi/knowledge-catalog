package index

import (
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestGitPushedReadmeCompilesIntoAccessSpecText(t *testing.T) {
	s := testkit.NewSetup(t, "kr://acme/payments")
	var schemaValue map[string]any
	for _, operation := range knowledge.SystemSchemaOperations() {
		if operation.Address.ObjectID == knowledge.CoreReadmeSchemaV1 {
			schemaValue = operation.Value.(map[string]any)
			break
		}
	}
	if schemaValue == nil {
		t.Fatal("readme schema is not published")
	}
	schemaed, err := s.Writer.Commit("readme-schema", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.CoreReadmeSchemaV1},
			Value: schemaValue,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, ok := snapshot.TreeStoreOf(s.Repo.Snapshot())
	if !ok {
		t.Fatal("tree store")
	}
	content := "---\nentity: payments\naspect: readme\nschema_ref: schema/core/readme/v1\n---\n# Payments warehouse\n\nPublished metrics.\n"
	head, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           schemaed.Result.CommitID,
		ExpectedTargetCommit: schemaed.Result.CommitID,
		Changes:              []snapshot.TreeChange{{Path: knowledge.RepositoryReadmePath, Content: []byte(content)}},
		Message:              "business git commit",
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.Repo.Read("payments", head)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := specAtCommit(s.Repo, head)
	if err != nil {
		t.Fatal(err)
	}
	hasText := false
	for _, field := range spec.Fields {
		if field.Schema == knowledge.CoreReadmeSchemaV1 && field.Path == "body" && field.Has(reader.HintText) {
			hasText = true
			break
		}
	}
	if !hasText {
		t.Fatalf("AccessSpec must include readme body text, got %#v", spec.Fields)
	}
	doc, include, err := compileValue(s.Repo, value, spec)
	if err != nil || !include {
		t.Fatalf("compile %v include=%v", err, include)
	}
	if !strings.Contains(doc.Text, "Payments warehouse") || !strings.Contains(doc.Text, "Published metrics") {
		t.Fatalf("SEARCH must compile git-pushed README body as text, got %q", doc.Text)
	}
}
