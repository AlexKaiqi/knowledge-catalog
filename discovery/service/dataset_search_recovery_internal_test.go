package service

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/kernel"
	"kc/object/reader"
)

// A rejected field names the describe call that shows which fields this
// Dataset revision can search, so the consumer can recover without guessing.
func TestDatasetSearchRejectionPointsToRevisionDescribe(t *testing.T) {
	run := &datasetSearchRun{
		req:  retrieval.SearchOf(retrieval.SearchMATCH("runbook")),
		pin:  reader.DatasetPin{SetID: "agent", Revision: 3},
		plan: retrieval.AccessPlan{Specs: []retrieval.AccessSpec{{Repository: "kr://acme/opaque", Commit: "c1"}}},
	}
	err := run.checkAccessContract()
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("member without text use accepted: %v", err)
	}
	for _, fact := range []string{"kr://acme/opaque", "index declaration selected into the dataset", "kc schema describe --dataset agent --revision 3"} {
		if !strings.Contains(err.Error(), fact) {
			t.Fatalf("rejection omits %q: %v", fact, err)
		}
	}
}

func TestDatasetSearchResultNamesTheRevisionItUsed(t *testing.T) {
	if basis := datasetBasis(reader.DatasetPin{SetID: "agent", Revision: 3}); basis == nil || basis.ID != "agent" || basis.Revision != 3 {
		t.Fatalf("named Dataset basis = %#v", basis)
	}
	if basis := datasetBasis(reader.DatasetPin{Revision: 1}); basis != nil {
		t.Fatalf("temporary definition reported a named revision: %#v", basis)
	}
}
