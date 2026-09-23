package reader

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func (r *treeRepository) Log(objectID knowledge.ObjectID, commit kernel.CommitID, query knowledge.ObjectLogQuery) ([]knowledge.ObjectRevision, error) {
	history, ok := snapshot.HistoryStoreOf(r.base)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no commit history", r.ID())
	}
	limit := query.PageSize()
	commits, err := history.CommitHistory(commit, 10000)
	if err != nil {
		return nil, err
	}
	unitsAt := make([][]repofile.Unit, len(commits))
	olderHasObject := make([]bool, len(commits)+1)
	for i := len(commits) - 1; i >= 0; i-- {
		unitsAt[i], err = r.objectUnits(objectID, commits[i])
		if err != nil {
			return nil, err
		}
		olderHasObject[i] = olderHasObject[i+1] || len(unitsAt[i]) > 0
	}
	var out []knowledge.ObjectRevision
	previous := ""
	var introducing *knowledge.ObjectRevision
	skipping := query.After != ""
	appendRevision := func(revision knowledge.ObjectRevision) bool {
		if skipping {
			if revision.Commit == query.After {
				skipping = false
			}
			return false
		}
		out = append(out, revision)
		return len(out) >= limit
	}
	for i, candidate := range commits {
		units := unitsAt[i]
		if len(units) == 0 {
			if !olderHasObject[i+1] {
				if introducing != nil {
					appendRevision(*introducing)
					introducing = nil
				}
				break
			}
			revision := knowledge.ObjectRevision{Commit: candidate, Status: knowledge.StatusRemoved}
			key := string(revision.Status)
			if key == previous {
				copyRevision := revision
				introducing = &copyRevision
				continue
			}
			if introducing != nil && appendRevision(*introducing) {
				return out, nil
			}
			previous = key
			copyRevision := revision
			introducing = &copyRevision
			continue
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
			if appendRevision(*introducing) {
				return out, nil
			}
		}
		previous = key
		copyRevision := revision
		introducing = &copyRevision
	}
	if introducing != nil && len(out) < limit {
		appendRevision(*introducing)
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
	changes, err := r.FastChangedObjects(from, to)
	if err != nil {
		return nil, err
	}
	out := make([]knowledge.ObjectID, 0, len(changes))
	for _, change := range changes {
		out = append(out, change.ObjectID)
	}
	return out, nil
}

func (r *treeRepository) FastChangedObjects(from, to kernel.CommitID) ([]knowledge.ChangedObject, error) {
	changes, ok := snapshot.ChangeStoreOf(r.base)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no changed-path scan", r.ID())
	}
	paths, err := changes.ChangedPaths(from, to)
	if err != nil {
		return nil, err
	}
	fromPaths := map[knowledge.ObjectID][]string{}
	toPaths := map[knowledge.ObjectID][]string{}
	seen := map[knowledge.ObjectID]struct{}{}
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		if objectID, ok := parseKnowledgeObjectAt(r.tree, path, from); ok {
			fromPaths[objectID] = append(fromPaths[objectID], path)
			seen[objectID] = struct{}{}
		}
		if objectID, ok := parseKnowledgeObjectAt(r.tree, path, to); ok {
			toPaths[objectID] = append(toPaths[objectID], path)
			seen[objectID] = struct{}{}
		}
	}
	out := make([]knowledge.ChangedObject, 0, len(seen))
	for objectID := range seen {
		out = append(out, knowledge.ChangedObject{
			ObjectID: objectID, FromPaths: fromPaths[objectID], ToPaths: toPaths[objectID],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ObjectID < out[j].ObjectID })
	return out, nil
}

func parseKnowledgeObjectAt(tree snapshot.TreeReader, path string, commit kernel.CommitID) (knowledge.ObjectID, bool) {
	content, err := tree.ReadFile(path, commit)
	if err != nil {
		return "", false
	}
	unit := repofile.Parse(string(content))
	if unit == nil {
		return "", false
	}
	return unit.ObjectID, true
}

func (r *treeRepository) ReadKnowledgeFile(rel string, commit kernel.CommitID) ([]byte, error) {
	return r.tree.ReadFile(rel, commit)
}

func (r *treeRepository) ReadManyAtPaths(objectIDs []knowledge.ObjectID, commit kernel.CommitID, paths map[knowledge.ObjectID][]string) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	seen := map[knowledge.ObjectID]struct{}{}
	ids := make([]knowledge.ObjectID, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		if objectID == "" {
			continue
		}
		if _, duplicate := seen[objectID]; duplicate {
			continue
		}
		seen[objectID] = struct{}{}
		ids = append(ids, objectID)
	}
	located, err := r.objectUnitPathsMany(ids, commit)
	if err != nil {
		return nil, err
	}
	for _, objectID := range ids {
		unitPaths := paths[objectID]
		if len(unitPaths) == 0 {
			unitPaths = located[objectID]
		}
		units, err := readObjectUnitsAtPaths(r.tree, unitPaths, objectID, commit)
		if err != nil {
			return nil, err
		}
		if len(units) == 0 {
			continue
		}
		value, err := assembleKnowledgeValue(r.ID(), objectID, commit, units)
		if err != nil {
			return nil, err
		}
		out[objectID] = value
	}
	return out, nil
}

// missingStatus classifies an absent object at a frozen basis for the
// object-level Resolve: REMOVED when the object existed in an earlier commit,
// UNRESOLVED when it never existed. Address-level resolution and Workspace
// unions treat both shapes as absent.
func (r *treeRepository) missingStatus(objectID knowledge.ObjectID, commit kernel.CommitID) (knowledge.ResolutionStatus, error) {
	history, ok := snapshot.HistoryStoreOf(r.base)
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
		units, err := r.objectUnits(objectID, prior)
		if err != nil {
			return "", err
		}
		if len(units) > 0 {
			return knowledge.StatusRemoved, nil
		}
	}
	return knowledge.StatusUnresolved, nil
}
