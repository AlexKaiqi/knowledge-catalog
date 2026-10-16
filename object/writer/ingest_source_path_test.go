package writer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/object/writer"
)

// DEBT-WRITER-SOURCE-PATH-01: directory publication must not record the
// caller's local layout in provenance. Without an explicit origin declaration
// the commit carries no provenance envelope at all, and the same material
// ingested from two different absolute directories produces identical change
// set messages (no machine-local directory in message or source refs).
func TestIngestDoesNotRecordLocalDirectoryInProvenance(t *testing.T) {
	material := func(t *testing.T) string {
		t.Helper()
		dir := testkit.TempDir(t)
		if err := os.WriteFile(filepath.Join(dir, "policy.md"), []byte("# Policy\nproduction requires a runbook"), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	messages := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		preview, err := writer.Ingest(material(t), "kr://acme/public/core", "P0")
		if err != nil {
			t.Fatal(err)
		}
		if preview.ChangeSet.Provenance != nil {
			t.Fatalf("directory publication without a declared origin must not carry provenance: %#v", preview.ChangeSet.Provenance)
		}
		messages = append(messages, preview.ChangeSet.Message)
	}
	if messages[0] != messages[1] {
		t.Fatalf("same material from different directories must produce a stable message: %q vs %q", messages[0], messages[1])
	}
	if strings.Contains(messages[0], "/") {
		t.Fatalf("commit message leaked a machine-local directory: %q", messages[0])
	}
}
