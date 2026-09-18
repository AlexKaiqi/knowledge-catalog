package testkit

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// ProviderParityContract applies one deterministic knowledge operation script
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
		operations []knowledge.Operation
		provenance *knowledge.ProvenanceEnvelope
	}
	steps := []step{
		{
			name: "put entity",
			operations: []knowledge.Operation{{
				Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"version": 1}, PathHint: "policies/A.json",
			}},
			provenance: &knowledge.ProvenanceEnvelope{OriginKind: knowledge.OriginSource, SourceRefs: []string{"source://handbook"}},
		},
		{
			name: "move and update entity",
			operations: []knowledge.Operation{{
				Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"version": 2}, PathHint: "archive/A.json",
			}},
		},
		{
			name: "compose independent aspects",
			operations: []knowledge.Operation{
				{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "dataset/T", AspectName: "structure"}, Value: map[string]any{"columns": 2}},
				{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "dataset/T", AspectName: "ownership"}, Value: map[string]any{"owner": "ops"}},
			},
		},
		{
			name: "put relation",
			operations: []knowledge.Operation{{
				Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "relation/contains"},
				PathHint: "relations/contains.json",
				Value: map[string]any{
					"relationId": "relation/contains", "relationType": "contains", "direction": "DIRECTED",
					"endpoints": []any{
						map[string]any{"role": "container", "objectRef": map[string]any{"repository": repositoryID, "object": "dataset/T"}},
						map[string]any{"role": "member", "objectRef": map[string]any{"repository": repositoryID, "object": "policy/A"}},
					},
				},
			}},
		},
		{
			name: "remove entity",
			operations: []knowledge.Operation{{
				Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
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
		if leftCode != rightCode || leftCode != kernel.ErrKnowledgeRefUnresolved {
			t.Fatalf("missing read codes differ: left=%s right=%s", leftCode, rightCode)
		}
	})
}

type paritySide struct {
	repo    knowledge.Repository
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

func (s *paritySide) apply(t *testing.T, index int, operations []knowledge.Operation, provenance *knowledge.ProvenanceEnvelope) {
	t.Helper()
	base := s.head()
	receipt, err := s.writer.Commit(parityCommand(index), knowledge.ChangeSet{
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
	_, err := s.writer.Commit("parity-stale", knowledge.ChangeSet{
		TargetRepository: s.repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: s.commits[0], ExpectedTargetCommit: s.commits[0],
		Operations: PutEntity("stale", 1, ""),
	})
	return kernel.CodeOf(err)
}

func parityCommand(index int) string {
	return "provider-parity-" + string(rune('0'+index))
}

func readMissing(repo knowledge.Repository, commit kernel.CommitID) error {
	_, err := repo.Read("missing", commit)
	return err
}

type parityObservation struct {
	Resolutions map[knowledge.ObjectID]knowledge.Resolution
	Values      map[knowledge.ObjectID]knowledge.KnowledgeValue
	Provenance  map[knowledge.ObjectID]knowledge.ProvenanceTrace
	ObjectIDs   []knowledge.ObjectID
	Changed     []knowledge.ObjectID
	History     map[knowledge.ObjectID][]parityRevision
}

type parityRevision struct {
	Step              int
	Status            knowledge.ResolutionStatus
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
		Resolutions: map[knowledge.ObjectID]knowledge.Resolution{},
		Values:      map[knowledge.ObjectID]knowledge.KnowledgeValue{},
		Provenance:  map[knowledge.ObjectID]knowledge.ProvenanceTrace{},
		History:     map[knowledge.ObjectID][]parityRevision{},
	}
	ids := []knowledge.ObjectID{"policy/A", "dataset/T", "relation/contains"}
	for _, id := range ids {
		resolution, err := side.repo.Resolve(id, current)
		if err != nil {
			t.Fatal(err)
		}
		resolution.Commit = ""
		out.Resolutions[id] = resolution
		if resolution.Status == knowledge.StatusResolved {
			value, err := side.repo.Read(id, current)
			if err != nil {
				t.Fatal(err)
			}
			value.Commit = ""
			sort.Slice(value.Units, func(i, j int) bool {
				return knowledge.AddressKey(value.Units[i]) < knowledge.AddressKey(value.Units[j])
			})
			sort.Slice(value.Declarations, func(i, j int) bool {
				return knowledge.AddressKey(value.Declarations[i].Address) <
					knowledge.AddressKey(value.Declarations[j].Address)
			})
			out.Values[id] = value
			trace, err := side.repo.GetProvenance(id, current)
			if err != nil {
				t.Fatal(err)
			}
			trace.Commit = ""
			out.Provenance[id] = trace
		}
		history, err := side.repo.Log(id, current, knowledge.ObjectLogQuery{})
		if err != nil {
			t.Fatal(err)
		}
		out.History[id] = normalizeHistory(history, side.commits)
	}
	scanner, err := knowledgemaintenance.RequireScanner(side.repo)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ScanSnapshotPage(current, knowledgemaintenance.ScanRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range page.Values {
		out.ObjectIDs = append(out.ObjectIDs, value.Address.ObjectID)
	}
	sort.Slice(out.ObjectIDs, func(i, j int) bool { return out.ObjectIDs[i] < out.ObjectIDs[j] })
	if len(side.commits) > 1 {
		fast, ok := side.repo.(knowledge.FastChanges)
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

func normalizeHistory(history []knowledge.ObjectRevision, commits []kernel.CommitID) []parityRevision {
	steps := make(map[kernel.CommitID]int, len(commits))
	for index, commit := range commits {
		steps[commit] = index
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
