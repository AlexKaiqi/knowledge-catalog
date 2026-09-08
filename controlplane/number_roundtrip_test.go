package controlplane_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"kc/controlplane"
	"kc/internal/testkit"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestPersistedProposalPreviewRetainsExactCandidateNumbers(t *testing.T) {
	s := setupLoop(t)
	proposal, err := s.CP.Propose(controlplane.ProposeInput{
		ProposalID: "large-number", RepositoryID: s.RepositoryID,
		TargetRef: snapshot.DefaultRef, CandidateRef: "refs/heads/candidates/large-number",
		BaseCommit: s.RootCommitID,
		Operations: testkit.PutEntity("sample/A", map[string]any{"first": int64(9007199254740992), "second": int64(9007199254740993)}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.CP.CreatePreview("maintenance", proposal)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "control-state.json")
	if err := controlplane.NewFileControlState(path).Save(controlplane.ControlState{
		Proposals: map[string]controlplane.Proposal{proposal.ProposalID: proposal},
		Previews:  map[string]controlplane.Preview{preview.PreviewID: preview},
	}); err != nil {
		t.Fatal(err)
	}
	restored, err := controlplane.NewFileControlState(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	fromProposal := restored.Proposals[proposal.ProposalID].CandidateCommit
	fromPreview := restored.Previews[preview.PreviewID].Repositories[s.RepositoryID]
	if fromProposal != fromPreview || fromProposal != proposal.CandidateCommit {
		t.Fatal("restored maintenance state changed candidate basis")
	}
	value, err := reader.NewReader(s.Store).Read(knowledge.KnowledgeRef{Repository: s.RepositoryID, Object: "sample/A"}, fromPreview, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value.Value)
	if err != nil || string(encoded) != `{"first":9007199254740992,"second":9007199254740993}` {
		t.Fatalf("restored proposal/preview changed authoritative numbers: %s, %v", encoded, err)
	}
	if testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != s.RootCommitID {
		t.Fatal("proposal advanced the published ref")
	}
}
