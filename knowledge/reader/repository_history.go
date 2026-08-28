package reader

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func (r *treeRepository) Log(objectID knowledge.ObjectID, commit kernel.CommitID, limit int) ([]knowledge.ObjectRevision, error) {
	history, ok := r.base.(snapshot.HistoryStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no commit history", r.ID())
	}
	if limit <= 0 {
		limit = 50
	}
	commits, err := history.CommitHistory(commit, 10000)
	if err != nil {
		return nil, err
	}
	var out []knowledge.ObjectRevision
	previous := ""
	var introducing *knowledge.ObjectRevision
	for _, candidate := range commits {
		units, err := readObjectUnits(r.tree, r.locator, objectID, candidate)
		if err != nil {
			return nil, err
		}
		if len(units) == 0 {
			if introducing != nil {
				out = append(out, *introducing)
				introducing = nil
			}
			break
		}
		value, err := assembleKnowledgeValue(r.ID(), objectID, candidate, units)
		if err != nil {
			return nil, err
		}
		revision := knowledge.ObjectRevision{
			Commit: candidate, Status: knowledge.StatusResolved,
			Digest:            kernel.CanonicalDigest(value.Value),
			DeclarationDigest: repofile.TreeDeclarationDigest(units),
		}
		key := string(revision.Status) + ":" + string(revision.Digest) + ":" + string(revision.DeclarationDigest)
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

func (r *treeRepository) Diff(objectID knowledge.ObjectID, from, to kernel.CommitID) (knowledge.ObjectDiff, error) {
	read := func(commit kernel.CommitID) (*knowledge.KnowledgeValue, error) {
		value, err := r.Read(objectID, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &value, nil
	}
	left, err := read(from)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	right, err := read(to)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	return knowledge.ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: left, To: right}, nil
}

func (r *treeRepository) FastChangedObjectIDs(from, to kernel.CommitID) ([]knowledge.ObjectID, error) {
	changes, ok := r.base.(snapshot.ChangeStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no changed-path scan", r.ID())
	}
	paths, err := changes.ChangedPaths(from, to)
	if err != nil {
		return nil, err
	}
	seen := map[knowledge.ObjectID]struct{}{}
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		for _, commit := range []kernel.CommitID{to, from} {
			content, err := r.tree.ReadFile(path, commit)
			if err != nil {
				continue
			}
			unit := repofile.Parse(string(content))
			if unit != nil {
				seen[unit.ObjectID] = struct{}{}
				break
			}
		}
	}
	out := make([]knowledge.ObjectID, 0, len(seen))
	for objectID := range seen {
		out = append(out, objectID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *treeRepository) missingStatus(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.ResolutionStatus, error) {
	history, ok := r.base.(snapshot.HistoryStore)
	if !ok {
		return knowledge.StatusUnresolved, nil
	}
	commits, err := history.CommitHistory(commit, 10000)
	if err != nil {
		return "", err
	}
	for _, prior := range commits {
		if prior == commit {
			continue
		}
		units, err := readObjectUnits(r.tree, r.locator, objectID, prior)
		if err != nil {
			return "", err
		}
		if len(units) > 0 {
			return knowledge.StatusRemoved, nil
		}
	}
	return knowledge.StatusUnresolved, nil
}
