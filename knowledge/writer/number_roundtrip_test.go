package writer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

func TestWriterReaderPreservesAdjacentLargeIntegers(t *testing.T) {
	s := testkit.NewSetup(t, "")
	schemaID := knowledge.ObjectID("schema/sample/value/v1")
	values := []int64{9007199254740992, 9007199254740993, 9223372036854775807}
	operations := []knowledge.Operation{{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: schemaID},
		Value: map[string]any{"entity": "Sample", "fields": map[string]any{"value": map[string]any{"type": "integer", "access": []any{"filter", "sort"}}}},
	}}
	for i, value := range values {
		operations = append(operations, knowledge.Operation{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.ObjectID("sample/" + string(rune('A'+i)))},
			SchemaRef: string(schemaID), Value: map[string]any{"value": value},
		})
	}
	published, err := s.Writer.Commit("publish-large-integers", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, original := range values {
		value, err := s.Reader.ReadAddress(s.RepositoryID, operations[i+1].Address, published.Result.CommitID)
		if err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(value.Value)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(map[string]any{"value": original})
		if string(got) != string(want) {
			t.Fatalf("Writer/Reader changed integer: got %s, want %s", got, want)
		}
		if kernel.CanonicalDigest(value.Value) != kernel.CanonicalDigest(operations[i+1].Value) {
			t.Fatal("authority round trip changed canonical digest")
		}
	}
}

func TestIngestJSONPreservesLargeIntegerValues(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"plain.json": `{"value":9007199254740993}`,
		"unit.json":  "---\nobject_id: sample/unit\n---\n{\"value\":9007199254740993}",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := writer.Ingest(dir, "kr://acme/public/core", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ChangeSet.Operations) != 2 {
		t.Fatalf("unexpected ingest operations: %#v", preview.ChangeSet.Operations)
	}
	for _, op := range preview.ChangeSet.Operations {
		got, _ := json.Marshal(op.Value)
		if string(got) != `{"value":9007199254740993}` {
			t.Fatalf("ingest rounded %s: %s", op.Address.ObjectID, got)
		}
	}
}
