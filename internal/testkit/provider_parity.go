package testkit

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

// ProviderParityContract applies one deterministic object operation script
// to two independent providers. Commits are aligned by script step because
// physical commit identifiers are intentionally provider-specific.
func ProviderParityContract(
	t *testing.T,
	leftFactory func(*testing.T, string) snapshot.Store,
	rightFactory func(*testing.T, string) snapshot.Store,
) {
	t.Helper()
	const repositoryID = "kr://conformance/provider-parity"
	left := newParitySide(t, leftFactory, repositoryID)
	right := newParitySide(t, rightFactory, repositoryID)

	type step struct {
		name       string
		operations []objectmodel.Operation
		provenance *objectmodel.ProvenanceEnvelope
	}
	steps := []step{
		{
			name: "put entity",
			operations: []objectmodel.Operation{{
				Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"version": 1}, PathHint: "policies/A.json",
			}},
			provenance: &objectmodel.ProvenanceEnvelope{OriginKind: objectmodel.OriginSource, SourceRefs: []string{"source://handbook"}},
		},
		{
			name: "move and update entity",
			operations: []objectmodel.Operation{{
				Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"version": 2}, PathHint: "archive/A.json",
			}},
		},
		{
			name: "compose independent aspects",
			operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "dataset/T", AspectName: "structure"}, Value: map[string]any{"columns": 2}},
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "dataset/T", AspectName: "ownership"}, Value: map[string]any{"owner": "ops"}},
			},
		},
		{
			name: "put relation",
			operations: []objectmodel.Operation{{
				Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation/contains"},
				PathHint: "relations/contains.json",
				Value: map[string]any{
					"relationId": "relation/contains", "relationType": "contains", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "container", "object": "dataset/T"},
						map[string]any{"role": "member", "object": "policy/A"},
					},
				},
			}},
		},
		{
			name: "update relation with cross-repository references",
			operations: []objectmodel.Operation{{
				Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation/contains"},
				Value: map[string]any{
					"relationId": "relation/contains", "relationType": "contains", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "container", "object": "dataset/T"},
						map[string]any{"role": "member", "object": "policy/A"},
					},
				},
			}},
		},
		{
			name: "remove entity",
			operations: []objectmodel.Operation{{
				Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			}},
		},
	}

	for index, item := range steps {
		t.Run(item.name, func(t *testing.T) {
			left.apply(t, index+1, item.operations, item.provenance)
			right.apply(t, index+1, item.operations, item.provenance)
			assertParityObservation(t, left, right)
		})
	}

	t.Run("failure codes", func(t *testing.T) {
		leftCode := left.staleWriteCode()
		rightCode := right.staleWriteCode()
		if leftCode != rightCode || leftCode != kernel.ErrNonFastForward {
			t.Fatalf("stale CAS codes differ: left=%s right=%s", leftCode, rightCode)
		}
		leftCode = kernel.CodeOf(readMissing(left.repo, left.head()))
		rightCode = kernel.CodeOf(readMissing(right.repo, right.head()))
		if leftCode != rightCode || leftCode != kernel.ErrObjectRefUnresolved {
			t.Fatalf("missing read codes differ: left=%s right=%s", leftCode, rightCode)
		}
	})
}

type paritySide struct {
	repo    objectmodel.Repository
	writer  *writer.Writer
	commits []kernel.CommitID
}

func newParitySide(t *testing.T, factory func(*testing.T, string) snapshot.Store, id string) *paritySide {
	t.Helper()
	raw := factory(t, id)
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registry.Close(); err != nil {
			t.Errorf("close parity provider: %v", err)
		}
	})
	repo, err := reader.NewReader(registry).Require(raw.ID(), kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	return &paritySide{repo: repo, writer: w, commits: []kernel.CommitID{root}}
}

func (s *paritySide) head() kernel.CommitID {
	return s.commits[len(s.commits)-1]
}

func (s *paritySide) apply(t *testing.T, index int, operations []objectmodel.Operation, provenance *objectmodel.ProvenanceEnvelope) {
	t.Helper()
	base := s.head()
	receipt, err := s.writer.Commit(parityCommand(index), objectmodel.ChangeSet{
		TargetRepository: s.repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Operations: operations, Provenance: provenance,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.commits = append(s.commits, receipt.Result.CommitID)
}

func (s *paritySide) staleWriteCode() kernel.ErrorCode {
	_, err := s.writer.Commit("parity-stale", objectmodel.ChangeSet{
		TargetRepository: s.repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: s.commits[0], ExpectedTargetCommit: s.commits[0],
		Operations: PutEntity("fixture/stale", 1, ""),
	})
	return kernel.CodeOf(err)
}

func parityCommand(index int) string {
	return "provider-parity-" + string(rune('0'+index))
}

func readMissing(repo objectmodel.Repository, commit kernel.CommitID) error {
	_, err := repo.Read("missing", commit)
	return err
}

type parityObservation struct {
	Resolutions map[objectmodel.ObjectID]objectmodel.Resolution
	Values      map[objectmodel.ObjectID]objectmodel.ObjectValue
	Provenance  map[objectmodel.ObjectID]objectmodel.ProvenanceTrace
	ObjectIDs   []objectmodel.ObjectID
	Changed     []objectmodel.ObjectID
	History     map[objectmodel.ObjectID][]parityRevision
}

type parityRevision struct {
	Step              int
	Status            objectmodel.ResolutionStatus
	Digest            kernel.Digest
	DeclarationDigest kernel.Digest
}

func assertParityObservation(t *testing.T, left, right *paritySide) {
	t.Helper()
	l := observeParity(t, left)
	r := observeParity(t, right)
	if !reflect.DeepEqual(l, r) {
		leftJSON, _ := json.MarshalIndent(l, "", "  ")
		rightJSON, _ := json.MarshalIndent(r, "", "  ")
		t.Fatalf("provider observations differ:\nleft:  %s\nright: %s", leftJSON, rightJSON)
	}
}

func observeParity(t *testing.T, side *paritySide) parityObservation {
	t.Helper()
	current := side.head()
	out := parityObservation{
		Resolutions: map[objectmodel.ObjectID]objectmodel.Resolution{},
		Values:      map[objectmodel.ObjectID]objectmodel.ObjectValue{},
		Provenance:  map[objectmodel.ObjectID]objectmodel.ProvenanceTrace{},
		History:     map[objectmodel.ObjectID][]parityRevision{},
	}
	ids := []objectmodel.ObjectID{"policy/A", "dataset/T", "relation/contains"}
	for _, id := range ids {
		resolution, err := side.repo.Resolve(id, current)
		if err != nil {
			t.Fatal(err)
		}
		resolution.Commit = ""
		out.Resolutions[id] = resolution
		if resolution.Status == objectmodel.StatusResolved {
			value, err := side.repo.Read(id, current)
			if err != nil {
				t.Fatal(err)
			}
			value.Commit = ""
			sort.Slice(value.Units, func(i, j int) bool {
				return objectmodel.AddressKey(value.Units[i]) < objectmodel.AddressKey(value.Units[j])
			})
			sort.Slice(value.Declarations, func(i, j int) bool {
				return objectmodel.AddressKey(value.Declarations[i].Address) <
					objectmodel.AddressKey(value.Declarations[j].Address)
			})
			out.Values[id] = value
			trace, err := side.repo.GetProvenance(id, current)
			if err != nil {
				t.Fatal(err)
			}
			trace.Commit = ""
			out.Provenance[id] = trace
		}
		history, err := side.repo.Log(id, current, objectmodel.ObjectLogQuery{})
		if err != nil {
			t.Fatal(err)
		}
		out.History[id] = normalizeHistory(history, side.commits)
	}
	scanner, err := objectmaintenance.RequireScanner(side.repo)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ScanSnapshotPage(current, objectmaintenance.ScanRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range page.Values {
		out.ObjectIDs = append(out.ObjectIDs, value.Address.ObjectID)
	}
	sort.Slice(out.ObjectIDs, func(i, j int) bool { return out.ObjectIDs[i] < out.ObjectIDs[j] })
	if len(side.commits) > 1 {
		fast, ok := side.repo.(objectmodel.FastChanges)
		if !ok {
			t.Fatalf("provider %T has no FastChanges capability", side.repo)
		}
		out.Changed, err = fast.FastChangedObjectIDs(side.commits[len(side.commits)-2], current)
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(out.Changed, func(i, j int) bool { return out.Changed[i] < out.Changed[j] })
	}
	return out
}

func normalizeHistory(history []objectmodel.ObjectRevision, commits []kernel.CommitID) []parityRevision {
	steps := make(map[kernel.CommitID]int, len(commits))
	for index, commit := range commits {
		if _, recorded := steps[commit]; !recorded {
			// A no-change command can reuse the prior commit. History names
			// that version's introduction, not the last command returning it.
			steps[commit] = index
		}
	}
	out := make([]parityRevision, 0, len(history))
	for _, revision := range history {
		out = append(out, parityRevision{
			Step: steps[revision.Commit], Status: revision.Status,
			Digest: revision.Digest, DeclarationDigest: revision.DeclarationDigest,
		})
	}
	return out
}
