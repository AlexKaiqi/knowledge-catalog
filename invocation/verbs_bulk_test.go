package invocation

import (
	"kc/internal/testkit"
	"kc/invoke"
	"kc/kernel"
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
