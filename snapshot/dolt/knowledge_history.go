package dolt

import (
	"strings"

	"kc/kernel"
)

func (r *DoltRepository) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, err := r.queryHash(string(commit)); err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if limit <= 0 {
		limit = 1000
	}
	commits, err := r.commitListLocked(string(commit))
	if err != nil {
		return nil, err
	}
	if len(commits) > limit {
		commits = commits[:limit]
	}
	return commits, nil
}

func (r *DoltRepository) commitListLocked(ref string) ([]kernel.CommitID, error) {
	out, err := r.run("log", "--oneline", ref)
	if err != nil {
		return nil, err
	}
	var commits []kernel.CommitID
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 0 {
			commits = append(commits, kernel.CommitID(fields[0]))
		}
	}
	return commits, nil
}
