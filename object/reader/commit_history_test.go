package reader

import (
	"kc/kernel"
	"testing"
)

type historyPageFunc func(kernel.CommitID, int) ([]kernel.CommitID, error)

func (f historyPageFunc) CommitHistory(c kernel.CommitID, n int) ([]kernel.CommitID, error) {
	return f(c, n)
}

func TestCommitHistoryWalkRejectsInvalidProviderPages(t *testing.T) {
	for name, page := range map[string][]kernel.CommitID{
		"empty": {}, "wrong-basis": {"other"}, "duplicate": {"start", "start"}, "empty-commit": {"start", ""},
	} {
		t.Run(name, func(t *testing.T) {
			visits := 0
			err := walkCommitHistory(historyPageFunc(func(kernel.CommitID, int) ([]kernel.CommitID, error) { return page, nil }), "start", func(kernel.CommitID) (bool, error) { visits++; return false, nil })
			if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || visits != 0 {
				t.Fatalf("invalid page: %v; visits=%d", err, visits)
			}
		})
	}
}
