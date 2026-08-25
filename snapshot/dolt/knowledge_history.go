package dolt

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

func (r *DoltRepository) Log(objectID knowledge.ObjectID, commit kernel.CommitID, limit int) ([]knowledge.ObjectRevision, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, err := r.queryHash(string(commit)); err != nil {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if limit <= 0 {
		limit = 50
	}
	commits, err := r.commitListLocked(string(commit))
	if err != nil {
		return nil, err
	}
	var out []knowledge.ObjectRevision
	previous := ""
	var introducing *knowledge.ObjectRevision
	for _, hash := range commits {
		resolution, err := r.resolveLocked(objectID, hash)
		if err != nil {
			return nil, err
		}
		if resolution.Status == knowledge.StatusUnresolved {
			if introducing != nil {
				out = append(out, *introducing)
			}
			break
		}
		key := string(resolution.Status) + ":" + string(resolution.Digest) + ":" + string(resolution.DeclarationDigest)
		revision := knowledge.ObjectRevision{Commit: hash, Status: resolution.Status, Digest: resolution.Digest, DeclarationDigest: resolution.DeclarationDigest}
		if key == previous {
			copyRevision := revision
			introducing = &copyRevision
			continue
		}
		if introducing != nil {
			out = append(out, *introducing)
			if len(out) >= limit {
				return out, nil
			}
		}
		previous = key
		copyRevision := revision
		introducing = &copyRevision
	}
	if introducing != nil && len(out) < limit {
		out = append(out, *introducing)
	}
	return out, nil
}

func (r *DoltRepository) Diff(objectID knowledge.ObjectID, from, to kernel.CommitID) (knowledge.ObjectDiff, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	readSide := func(commit kernel.CommitID) (*knowledge.KnowledgeValue, error) {
		resolution, err := r.resolveLocked(objectID, commit)
		if err != nil {
			return nil, err
		}
		if resolution.Status != knowledge.StatusResolved {
			return nil, nil
		}
		value, err := r.readLocked(objectID, commit)
		return &value, err
	}
	left, err := readSide(from)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	right, err := readSide(to)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	return knowledge.ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: left, To: right}, nil
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

func (r *DoltRepository) everExistedLocked(objectID knowledge.ObjectID) bool {
	commits, err := r.commitListLocked("--all")
	if err != nil {
		return false
	}
	for _, commit := range commits {
		tree, err := r.scanAtLocked(commit)
		if err == nil && len(tree.ObjectUnits(objectID)) > 0 {
			return true
		}
	}
	return false
}
