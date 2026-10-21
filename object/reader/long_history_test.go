package reader_test

import (
	"fmt"
	"strconv"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

// A synthetic immutable chain keeps the >10000 boundary cheap while exercising
// the production tree interpreter, including canonical units and state digests.
type longHistoryStore struct {
	snapshot.Store
	snapshot.TreeReader
	state        func(int) string
	readErrorAt  int
	reads        []int
	historyCalls []kernel.CommitID
	pageCap      int
	schemaAt     func(int) string
}

func (*longHistoryStore) ID() kernel.RepositoryID        { return "kr://test/long-history" }
func (*longHistoryStore) HasCommit(kernel.CommitID) bool { return true }
func historyCommit(n int) kernel.CommitID                { return kernel.CommitID(fmt.Sprintf("c%05d", n)) }
func historyNumber(c kernel.CommitID) int                { n, _ := strconv.Atoi(string(c)[1:]); return n }
func (s *longHistoryStore) CommitHistory(start kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	s.historyCalls = append(s.historyCalls, start)
	if s.pageCap > 0 {
		limit = min(limit, s.pageCap)
	}
	out := []kernel.CommitID{}
	for n := historyNumber(start); n >= 0 && len(out) < limit; n-- {
		out = append(out, historyCommit(n))
	}
	return out, nil
}
func (s *longHistoryStore) ObjectUnitPaths(_ objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	if s.state(historyNumber(at)) == "" {
		return nil, nil
	}
	return []string{"notes/history.yaml"}, nil
}
func (s *longHistoryStore) ReadFile(_ string, at kernel.CommitID) ([]byte, error) {
	n := historyNumber(at)
	s.reads = append(s.reads, n)
	if n == s.readErrorAt {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "history read unavailable")
	}
	schema := ""
	if s.schemaAt != nil {
		schema = "schema_ref: " + s.schemaAt(n) + "\n"
	}
	return []byte("---\nobject_id: note/history\npath_hint: notes/history.yaml\n" + schema + "---\nbody: " + s.state(n) + "\n"), nil
}
func longHistoryRepository(t *testing.T, state func(int) string) (*longHistoryStore, objectmodel.Repository) {
	t.Helper()
	store := &longHistoryStore{state: state, readErrorAt: -1}
	repo, err := reader.NewReader(nil).Wrap(store, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	return store, repo
}

func TestLongObjectHistoryPreservesRealIntroductionsAndPages(t *testing.T) {
	_, repo := longHistoryRepository(t, func(n int) string {
		switch {
		case n < 4:
			return ""
		case n < 6:
			return "A"
		case n < 10019:
			return "B"
		default:
			return "A"
		}
	})
	after := kernel.CommitID("")
	for _, want := range []int{10019, 6, 4} {
		page, err := repo.Log("note/history", historyCommit(10020), objectmodel.ObjectLogQuery{Limit: 1, After: after})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 1 || page[0].Commit != historyCommit(want) || page[0].Status != objectmodel.StatusResolved {
			t.Fatalf("after %s: revisions = %#v, want introduction %s", after, page, historyCommit(want))
		}
		after = page[0].Commit
	}
	page, err := repo.Log("note/history", historyCommit(10020), objectmodel.ObjectLogQuery{Limit: 1, After: after})
	if err != nil || len(page) != 0 {
		t.Fatalf("exhausted history = %#v, %v", page, err)
	}
}

func TestLongObjectHistoryDoesNotInventWindowIntroduction(t *testing.T) {
	_, repo := longHistoryRepository(t, func(n int) string {
		if n < 5 {
			return ""
		}
		return "A"
	})
	page, err := repo.Log("note/history", historyCommit(10020), objectmodel.ObjectLogQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].Commit != historyCommit(5) {
		t.Fatalf("history = %#v, want true introduction c00005", page)
	}
}

func TestLongObjectHistoryFindsRemovalBeyondOldWindow(t *testing.T) {
	_, repo := longHistoryRepository(t, func(n int) string {
		if n >= 4 && n < 6 {
			return "A"
		}
		return ""
	})
	page, err := repo.Log("note/history", historyCommit(10020), objectmodel.ObjectLogQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Commit != historyCommit(6) || page[0].Status != objectmodel.StatusRemoved || page[1].Commit != historyCommit(4) {
		t.Fatalf("removal history = %#v", page)
	}
	resolved, err := repo.Resolve("note/history", historyCommit(10020))
	if err != nil || resolved.Status != objectmodel.StatusRemoved {
		t.Fatalf("old removal resolve = %#v, %v", resolved, err)
	}
}

func TestObjectHistoryStopsAfterProvingRequestedRevision(t *testing.T) {
	store, repo := longHistoryRepository(t, func(n int) string {
		if n >= 8 {
			return "B"
		}
		return "A"
	})
	store.readErrorAt = 2
	page, err := repo.Log("note/history", historyCommit(10), objectmodel.ObjectLogQuery{Limit: 1})
	if err != nil || len(page) != 1 || page[0].Commit != historyCommit(8) {
		t.Fatalf("first history page = %#v, %v", page, err)
	}
	if len(store.reads) > 4 {
		t.Fatalf("read unrelated older states: %v", store.reads)
	}
	_, err = repo.Log("note/history", historyCommit(10), objectmodel.ObjectLogQuery{Limit: 2})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("older failure = %v", err)
	}
}

func TestObjectHistoryPreservesRecreationAndTrueRoot(t *testing.T) {
	_, repo := longHistoryRepository(t, func(n int) string {
		if n >= 4 && n < 7 {
			return ""
		}
		return "A"
	})
	page, err := repo.Log("note/history", historyCommit(10), objectmodel.ObjectLogQuery{})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{7, 4, 0}
	if len(page) != len(want) {
		t.Fatalf("recreation history = %#v", page)
	}
	for i, n := range want {
		if page[i].Commit != historyCommit(n) {
			t.Fatalf("revision %d = %#v", i, page[i])
		}
	}
	if page[1].Status != objectmodel.StatusRemoved {
		t.Fatalf("deletion missing: %#v", page)
	}
}

func TestObjectHistoryShortProviderPagesDoNotProveIntroduction(t *testing.T) {
	store, repo := longHistoryRepository(t, func(n int) string {
		if n < 5 {
			return ""
		}
		return "A"
	})
	store.pageCap = 3
	page, err := repo.Log("note/history", historyCommit(20), objectmodel.ObjectLogQuery{})
	if err != nil || len(page) != 1 || page[0].Commit != historyCommit(5) {
		t.Fatalf("short-page introduction = %#v, %v", page, err)
	}
}

func TestObjectHistoryPreservesDeclarationOnlyChangesAcrossBatches(t *testing.T) {
	store, repo := longHistoryRepository(t, func(n int) string {
		if n < 4 {
			return ""
		}
		return "A"
	})
	store.schemaAt = func(n int) string {
		if n >= 510 {
			return "schema/history-new"
		}
		return "schema/history-old"
	}
	page, err := repo.Log("note/history", historyCommit(520), objectmodel.ObjectLogQuery{})
	if err != nil || len(page) != 2 || page[0].Commit != historyCommit(510) || page[1].Commit != historyCommit(4) {
		t.Fatalf("declaration history = %#v, %v", page, err)
	}
	if page[0].Digest != page[1].Digest || page[0].DeclarationDigest == page[1].DeclarationDigest {
		t.Fatalf("declaration-only change lost: %#v", page)
	}
}
