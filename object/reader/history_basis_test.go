package reader_test

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"testing"
)

func TestHistoryBasisPreservesWholeSourceWithoutOpeningInterpretation(t *testing.T) {
	_, guard, _, at := newReferenceReadFixture(t)
	source, err := reader.NewReader(nil).Wrap(&basisSnapshot{referenceReadGuard: guard, binding: "history-origin"}, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	basis := objectmodel.ReadBasis{Version: 1, Reference: "urn:kc:object:table/orders", Candidates: []objectmodel.ReadSource{{Repository: source.ID(), Commit: at, Binding: source.(objectmodel.StoreIdentified).StoreDigest()}}, Paths: []objectmodel.ReadPath{{Repository: source.ID(), Kind: "prefix"}, {Repository: source.ID(), Kind: "file", File: "public/id.yaml"}}, Dependencies: []objectmodel.ReadSource{{Repository: "kr://unavailable/interpretation", Commit: "missing", Binding: "old-interpretation"}}, UnresolvedDependencies: []objectmodel.ReadDependencyFailure{{Repository: "kr://unavailable/interpretation", Code: kernel.ErrVersionUnresolved}}}
	calls := 0
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		calls++
		if id != source.ID() {
			t.Fatalf("history opened interpretation source %s", id)
		}
		return source, nil
	}
	if _, err := reader.OpenHistoryBasis(lookup, basis); err != nil || calls != 1 || len(guard.commits) != 0 {
		t.Fatalf("whole history range lost to extra delivery path or interpretation: calls=%d bodyReads=%v error=%v", calls, guard.commits, err)
	}
	basis.Paths = basis.Paths[1:]
	calls = 0
	if _, err := reader.OpenHistoryBasis(lookup, basis); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || calls != 0 {
		t.Fatalf("file-only history escaped scope before source access: %v calls=%d", err, calls)
	}
}
