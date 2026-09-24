package cli

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
)

// LAKEFS-04: --bulk belongs to the writer commit surface only. Single-
// operation writes and dataset commits reject it instead of silently
// ignoring the flag, and the ChangeSet wire keeps carrying bulkIngest.

func TestBulkFlagRejectedOnSingleOperationWrites(t *testing.T) {
	_, err := verbRemove(&invocation{Flags: map[string]FlagValue{"object": "metric/gmv", "bulk": true}})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestBulkFlagRejectedOnDatasetCommit(t *testing.T) {
	_, err := verbCommit(&invocation{Flags: map[string]FlagValue{"dataset": "kr://acme/catalog/ds", "bulk": true}})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

func TestDecodeChangeSetCarriesBulkIngestFromWire(t *testing.T) {
	cs, err := decodeChangeSet([]byte(`{"targetRepository":"kr://acme/x","operations":[],"bulkIngest":true}`), "wire")
	if err != nil {
		t.Fatal(err)
	}
	if !cs.BulkIngest {
		t.Fatal("bulkIngest was dropped while decoding the wire ChangeSet")
	}
	plain, err := decodeChangeSet([]byte(`{"targetRepository":"kr://acme/x","operations":[]}`), "wire")
	if err != nil {
		t.Fatal(err)
	}
	if plain.BulkIngest {
		t.Fatal("absent bulkIngest must decode as the staging path")
	}
}
