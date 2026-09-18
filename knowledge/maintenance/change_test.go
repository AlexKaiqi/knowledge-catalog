package maintenance

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

type failingFastChangesRepository struct {
	knowledge.Repository
	scans int
}

func (r *failingFastChangesRepository) FastChangedObjectIDs(kernel.CommitID, kernel.CommitID) ([]knowledge.ObjectID, error) {
	return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "native change lookup failed")
}

func (r *failingFastChangesRepository) ObjectIDsPage(kernel.CommitID, int, string) (knowledge.ObjectIDPage, error) {
	r.scans++
	return knowledge.ObjectIDPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "full scan must not run")
}

func (r *failingFastChangesRepository) ReadMany([]knowledge.ObjectID, kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	return nil, kernel.Fail(kernel.ErrPreconditionFailed, "full scan hydrate must not run")
}

func TestChangedObjectIDsFailsClosedWhenNativeChangesFail(t *testing.T) {
	repo := &failingFastChangesRepository{}
	_, err := ChangedObjectIDs(repo, "from", "to")
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("native change error must be preserved, got %v", err)
	}
	if repo.scans != 0 {
		t.Fatalf("native change failure triggered %d maintenance scans", repo.scans)
	}
}
