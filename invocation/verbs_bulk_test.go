package invocation

import (
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBulkFlagRejectedOnSingleOperationWrites(t *testing.T) {
	_, err := VerbRemove(&Invocation{Flags: map[string]invoke.FlagValue{"reference": "urn:kc:object:metric/gmv", "command-id": "reject-bulk", "bulk": true}})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if !strings.Contains(err.Error(), "single object maintenance") {
		t.Fatalf("bulk refusal was masked by a missing input: %v", err)
	}
}

func TestDesiredDirectoryPreservesBulkThroughUnchangedFilter(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"same.json":    "---\nobject_id: notes/same\n---\n{\"v\":1}\n",
		"changed.json": "---\nobject_id: notes/changed\n---\n{\"v\":2}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, bulk := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "bulk"}[bulk], func(t *testing.T) {
			flags := map[string]invoke.FlagValue{}
			if bulk {
				flags["bulk"] = true
			}
			preview, err := IngestDesired(flags, dir, "kr://test/directory", snapshot.DefaultRef, "basis")
			if err != nil {
				t.Fatal(err)
			}
			address := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "notes/same"}
			current := map[string]string{objectmodel.AddressKey(address): string(kernel.CanonicalDigest(map[string]any{"v": 1}))}
			filtered, summary := writer.OmitUnchanged(preview.ChangeSet, current)
			if summary.Unchanged != 1 || len(filtered.Operations) != 1 || filtered.Operations[0].Address.ObjectID != "notes/changed" {
				t.Fatalf("directory filter lost its comparison: summary=%#v operations=%#v", summary, filtered.Operations)
			}
			if filtered.BulkIngest != bulk {
				t.Fatalf("directory bulk option was dropped: got %v, want %v", filtered.BulkIngest, bulk)
			}
		})
	}
}

func TestBulkFlagRejectedOnDatasetCommit(t *testing.T) {
	_, err := VerbCommit(&Invocation{Flags: map[string]invoke.FlagValue{"dataset": "kr://acme/catalog/ds", "bulk": true}})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestDecodeChangeSetCarriesBulkIngestFromWire(t *testing.T) {
	cs, err := DecodeChangeSet([]byte(`{"targetRepository":"kr://acme/x","operations":[],"bulkIngest":true}`), "wire")
	if err != nil {
		t.Fatal(err)
	}
	if !cs.BulkIngest {
		t.Fatal("bulkIngest was dropped while decoding the wire ChangeSet")
	}
	plain, err := DecodeChangeSet([]byte(`{"targetRepository":"kr://acme/x","operations":[]}`), "wire")
	if err != nil {
		t.Fatal(err)
	}
	if plain.BulkIngest {
		t.Fatal("absent bulkIngest must decode as the staging path")
	}
}
