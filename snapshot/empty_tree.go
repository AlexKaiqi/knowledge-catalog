package snapshot

import "kc/kernel"

// ProbeEmptyTree reports whether an immutable tree has no literal files or
// directories. A provider without DirectoryReader reports known=false; the
// probe never falls back to an unbounded ListFiles scan. Hidden paths count as
// content too: absence of an object index cannot prove absence of files.
func ProbeEmptyTree(store Store, commit kernel.CommitID) (empty bool, known bool, err error) {
	if commit == "" {
		return false, false, kernel.Fail(kernel.ErrUsageInvalid, "empty tree probe requires a fixed commit")
	}
	directory, ok := DirectoryReaderOf(store)
	if !ok {
		return false, false, nil
	}
	request := DirectoryRequest{Commit: commit, Limit: 1}
	seen := map[string]bool{"": true}
	for {
		page, err := directory.ReadDirectory(request)
		if err != nil {
			return false, false, err
		}
		if page.Generation != string(commit) || len(page.Entries) > request.Limit {
			return false, false, kernel.Fail(kernel.ErrPreconditionFailed, "empty tree probe returned invalid fixed directory evidence")
		}
		if len(page.Entries) != 0 {
			return false, true, nil
		}
		if page.Exhausted {
			if page.Continuation != "" {
				return false, false, kernel.Fail(kernel.ErrPreconditionFailed, "exhausted empty tree probe retained a continuation")
			}
			return true, true, nil
		}
		if page.Continuation == "" || seen[page.Continuation] {
			return false, false, kernel.Fail(kernel.ErrPreconditionFailed, "empty tree probe directory did not advance")
		}
		seen[page.Continuation] = true
		request.Continuation = page.Continuation
	}
}
