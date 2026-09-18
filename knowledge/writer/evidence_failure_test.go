package writer_test

import (
	"errors"
	"testing"

	"kc/internal/journal"
	"kc/internal/testkit"
	"kc/knowledge"
	"kc/snapshot"
)

type failingEvidenceJournal struct{}

func (failingEvidenceJournal) Record(journal.Event) error {
	return errors.New("evidence medium unavailable")
}

func TestAcceptedCommitSurvivesEvidenceFailure(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://writer/evidence-failure")
	setup.Writer.SetJournal(failingEvidenceJournal{})
	receipt, err := setup.Writer.Commit("accepted", knowledge.ChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: testkit.PutEntity("policy/A", map[string]any{"version": 1}, ""),
	})
	if err != nil {
		t.Fatalf("accepted Canonical commit was reported as failed: %v", err)
	}
	head := testkit.MustHead(t, setup.Repo, snapshot.DefaultRef)
	if receipt.Result.CommitID == "" || head != receipt.Result.CommitID {
		t.Fatalf("accepted result was lost: receipt=%#v head=%s", receipt, head)
	}
}
