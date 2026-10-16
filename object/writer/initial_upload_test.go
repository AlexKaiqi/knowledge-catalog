package writer_test

import (
	"fmt"
	"sync"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/writer"
	"kc/snapshot"
)

// initialUploadStore counts actual authority reads, not calls satisfied by
// Writer's immutable-basis planning view.
type initialUploadStore struct {
	snapshot.Store
	snapshot.TreeStore
	directory snapshot.DirectoryReader
	bulk      snapshot.BulkTreeIngester
	basis     kernel.CommitID
	mu        sync.Mutex
	reads     int
	oldReads  int
	lists     int
	probes    int
	bulkCalls int
	lastApply snapshot.TreeChangeSet
}

func (s *initialUploadStore) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	s.mu.Lock()
	if at == s.basis {
		s.reads++
	} else {
		s.oldReads++
	}
	s.mu.Unlock()
	return s.TreeStore.ReadFile(path, at)
}

func (s *initialUploadStore) ListFiles(at kernel.CommitID) ([]string, error) {
	s.lists++
	return s.TreeStore.ListFiles(at)
}

func (s *initialUploadStore) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	s.probes++
	if request.Commit != s.basis || request.Directory != "" || request.Limit != 1 {
		return snapshot.DirectoryPage{}, fmt.Errorf("unbounded or unfixed empty probe: %#v", request)
	}
	return s.directory.ReadDirectory(request)
}

func (s *initialUploadStore) ApplyTreeCommitBulk(cs snapshot.TreeChangeSet) (kernel.CommitID, error) {
	s.bulkCalls++
	s.lastApply = cs
	return s.bulk.ApplyTreeCommitBulk(cs)
}

func initialUploadWriter(t *testing.T) (*initialUploadStore, *writer.Writer) {
	t.Helper()
	raw := testkit.MakeTreeStore(t, "kr://writer/initial-upload")
	s := &initialUploadStore{
		Store: raw, TreeStore: raw.(snapshot.TreeStore),
		directory: raw.(snapshot.DirectoryReader), bulk: raw.(snapshot.BulkTreeIngester),
		basis: testkit.MustHead(t, raw, snapshot.DefaultRef),
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(s); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, w
}

func initialUploadChanges(s *initialUploadStore, count int) objectmodel.ChangeSet {
	operations := []objectmodel.Operation{{
		Op:      objectmodel.OpPut,
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/initial-note"},
		Value: map[string]any{"entity": "InitialNote", "fields": map[string]any{
			"label": map[string]any{"type": "string", "required": true},
		}},
	}}
	for index := 0; index < count; index++ {
		operations = append(operations, objectmodel.Operation{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(fmt.Sprintf("note/%d", index))},
			SchemaRef: "schema/initial-note", PathHint: fmt.Sprintf("notes/%d.json", index),
			Value: map[string]any{"label": fmt.Sprintf("note %d", index)},
		})
	}
	return objectmodel.ChangeSet{
		TargetRepository: s.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: s.basis, ExpectedTargetCommit: s.basis,
		Operations: operations, BulkIngest: true,
	}
}

func TestInitialUploadUsesBulkWithoutReadingOldUnits(t *testing.T) {
	for _, ref := range []string{snapshot.DefaultRef, "refs/heads/review/initial"} {
		t.Run(ref, func(t *testing.T) {
			s, w := initialUploadWriter(t)
			if ref != snapshot.DefaultRef {
				if err := s.CreateRef(ref, s.basis); err != nil {
					t.Fatal(err)
				}
			}
			cs := initialUploadChanges(s, 64)
			cs.TargetRef = ref
			receipt, err := w.Commit("initial-bulk", cs)
			if err != nil {
				t.Fatal(err)
			}
			if s.reads != 0 || s.lists != 0 || s.probes != 1 {
				t.Fatalf("empty-basis upload consulted old units: reads=%d lists=%d probes=%d", s.reads, s.lists, s.probes)
			}
			if s.bulkCalls != 1 || s.lastApply.TargetRef != ref || s.lastApply.BaseCommit != s.basis || s.lastApply.ExpectedTargetCommit != s.basis {
				t.Fatalf("initial apply lost bulk or fixed target coordinates: calls=%d apply=%#v", s.bulkCalls, s.lastApply)
			}
			raw, err := s.TreeStore.ReadFile("notes/63.json", receipt.Result.NewCommit)
			if err != nil || repofile.Parse(string(raw)) == nil {
				t.Fatalf("bulk upload did not publish readable units: %s %v", raw, err)
			}
			replay, err := w.Commit("initial-bulk", cs)
			if err != nil || replay.Disposition != writer.DispositionReplayed || replay.Result.NewCommit != receipt.Result.NewCommit || s.bulkCalls != 1 || s.probes != 1 {
				t.Fatalf("initial replay touched authority again: %#v err=%v bulk=%d probes=%d", replay, err, s.bulkCalls, s.probes)
			}
		})
	}
}

func TestInitialUploadKeepsValidationAndCAS(t *testing.T) {
	for _, test := range []struct {
		name string
		code kernel.ErrorCode
		edit func(*initialUploadStore, *objectmodel.ChangeSet)
	}{
		{"invalid instance", kernel.ErrSchemaInstanceInvalid, func(_ *initialUploadStore, cs *objectmodel.ChangeSet) {
			cs.Operations[1].Value = map[string]any{"label": 42}
		}},
		{"same batch path collision", kernel.ErrObjectIDConflict, func(_ *initialUploadStore, cs *objectmodel.ChangeSet) {
			cs.Operations[2].PathHint = cs.Operations[1].PathHint
		}},
		{"missing Schema", kernel.ErrSchemaRevisionUnresolved, func(_ *initialUploadStore, cs *objectmodel.ChangeSet) {
			cs.Operations = cs.Operations[1:]
		}},
		{"invalid Schema", kernel.ErrSchemaUnsupported, func(_ *initialUploadStore, cs *objectmodel.ChangeSet) {
			cs.Operations[0].Value = map[string]any{"entity": "InitialNote", "fields": map[string]any{"label": map[string]any{"type": "invented"}}}
		}},
		{"stale CAS", kernel.ErrNonFastForward, func(s *initialUploadStore, _ *objectmodel.ChangeSet) {
			_, err := s.TreeStore.ApplyTreeCommit(snapshot.TreeChangeSet{
				TargetRepository: s.ID(), TargetRef: snapshot.DefaultRef,
				BaseCommit: s.basis, ExpectedTargetCommit: s.basis,
				Changes: []snapshot.TreeChange{{Path: "concurrent.txt", Content: []byte("concurrent")}},
			})
			if err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, w := initialUploadWriter(t)
			cs := initialUploadChanges(s, 2)
			test.edit(s, &cs)
			before := testkit.MustHead(t, s, snapshot.DefaultRef)
			_, err := w.Commit("refused-initial", cs)
			testkit.ExpectCode(t, err, test.code)
			if s.reads != 0 || s.lists != 0 || testkit.MustHead(t, s, snapshot.DefaultRef) != before {
				t.Fatalf("refused initial upload read old units or changed head: reads=%d lists=%d", s.reads, s.lists)
			}
		})
	}
}

func TestNonemptyInitialBasisKeepsOldUnitReads(t *testing.T) {
	s, w := initialUploadWriter(t)
	seed, err := w.Commit("seed-initial", initialUploadChanges(s, 2))
	if err != nil {
		t.Fatal(err)
	}
	s.basis = seed.Result.NewCommit
	s.reads, s.lists, s.probes = 0, 0, 0
	cs := initialUploadChanges(s, 2)
	cs.Operations[1].Value = map[string]any{"label": "updated"}
	if _, err := w.Commit("update-initial", cs); err != nil {
		t.Fatal(err)
	}
	if s.reads == 0 || s.probes != 1 || s.lists != 0 {
		t.Fatalf("nonempty basis lost incremental old-unit checks: reads=%d lists=%d probes=%d", s.reads, s.lists, s.probes)
	}
}

func TestInitialUploadReadsExplicitHistoricalSchema(t *testing.T) {
	s, w := initialUploadWriter(t)
	seed, err := w.Commit("historical-schema", initialUploadChanges(s, 1))
	if err != nil {
		t.Fatal(err)
	}
	ref := "refs/heads/review/historical-schema"
	if err := s.CreateRef(ref, s.basis); err != nil {
		t.Fatal(err)
	}
	cs := initialUploadChanges(s, 1)
	cs.TargetRef = ref
	cs.Operations = cs.Operations[1:]
	cs.Operations[0].SchemaRef += "@" + string(seed.Result.NewCommit)
	s.reads, s.oldReads, s.probes = 0, 0, 0
	if _, err := w.Commit("initial-with-history", cs); err != nil {
		t.Fatalf("empty current basis masked a pinned historical Schema: %v", err)
	}
	if s.reads != 0 || s.oldReads == 0 || s.probes != 1 {
		t.Fatalf("empty cache escaped its immutable basis: current reads=%d historical reads=%d probes=%d", s.reads, s.oldReads, s.probes)
	}
}

func TestEmptyBaseDoesNotHideDifferentExpectedSchemaBasis(t *testing.T) {
	s, w := initialUploadWriter(t)
	seed, err := w.Commit("schema-at-expected", initialUploadChanges(s, 1))
	if err != nil {
		t.Fatal(err)
	}
	cs := initialUploadChanges(s, 1)
	cs.ExpectedTargetCommit = seed.Result.NewCommit
	cs.Operations[0].Value = map[string]any{"entity": "InitialNote", "fields": map[string]any{"label": map[string]any{"type": "number", "required": true}}}
	cs.Operations[1].Value = map[string]any{"label": 42}
	s.reads, s.oldReads, s.probes = 0, 0, 0
	_, err = w.Commit("incompatible-at-expected", cs)
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if s.reads != 0 || s.oldReads == 0 || testkit.MustHead(t, s, snapshot.DefaultRef) != seed.Result.NewCommit {
		t.Fatalf("empty base bypassed checks at a different expected basis: current reads=%d expected reads=%d", s.reads, s.oldReads)
	}
}

func TestInitialUploadWithoutDirectoryCapabilityKeepsExistingChecks(t *testing.T) {
	s, _ := initialUploadWriter(t)
	// The interface fields intentionally hide DirectoryReader and bulk.
	store := &stagingOnlyStore{Store: s.Store, tree: s}
	registry := snapshot.NewRegistry()
	if err := registry.Add(store); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs := initialUploadChanges(s, 2)
	cs.BulkIngest = false
	if _, err := w.Commit("unknown-directory", cs); err != nil {
		t.Fatal(err)
	}
	if s.reads == 0 || s.lists != 0 || s.probes != 0 {
		t.Fatalf("unsupported empty probe skipped old checks or scanned: reads=%d lists=%d probes=%d", s.reads, s.lists, s.probes)
	}
}

func TestEmptyBasisDoesNotInventBulkCapability(t *testing.T) {
	s, _ := initialUploadWriter(t)
	store := struct {
		snapshot.Store
		snapshot.TreeStore
		snapshot.DirectoryReader
	}{s.Store, s, s}
	registry := snapshot.NewRegistry()
	if err := registry.Add(store); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("empty-without-bulk", initialUploadChanges(s, 2))
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
	if s.reads != 0 || s.lists != 0 || s.bulkCalls != 0 || testkit.MustHead(t, s, snapshot.DefaultRef) != s.basis {
		t.Fatalf("empty-basis read view changed write capability: reads=%d lists=%d bulk=%d", s.reads, s.lists, s.bulkCalls)
	}
}
