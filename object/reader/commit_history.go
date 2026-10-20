package reader

import (
	"kc/kernel"
	"kc/snapshot"
)

const commitHistoryPageSize = 256

// Visit the fixed first-parent chain using overlapping immutable starts. A
// short provider page does not prove exhaustion: probe again from its last
// commit until it has no older parent. The callback stops once enough revisions
// have been established.
func walkCommitHistory(history snapshot.HistoryStore, start kernel.CommitID, visit func(kernel.CommitID) (bool, error)) error {
	seen := map[kernel.CommitID]bool{}
	first := true
	for {
		if seen[start] {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "commit history did not advance")
		}
		seen[start] = true
		page, err := history.CommitHistory(start, commitHistoryPageSize)
		if err != nil {
			return err
		}
		if len(page) == 0 || page[0] != start || len(page) > commitHistoryPageSize {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "commit history did not return its requested basis")
		}
		pageSeen := map[kernel.CommitID]bool{}
		for _, commit := range page {
			if commit == "" || pageSeen[commit] {
				return kernel.Fail(kernel.ErrTemporaryUnavailable, "commit history returned an invalid chain")
			}
			pageSeen[commit] = true
		}
		offset := 0
		if !first {
			offset = 1
		}
		for _, commit := range page[offset:] {
			stop, err := visit(commit)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		if len(page) == 1 {
			return nil
		}
		start = page[len(page)-1]
		first = false
	}
}
