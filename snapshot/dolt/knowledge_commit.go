package dolt

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

func (r *DoltRepository) scanAtLocked(commit kernel.CommitID) (*repofile.Tree, error) {
	files, err := r.snapshotFiles(commit)
	if err != nil {
		return nil, err
	}
	tree := repofile.NewTree()
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		unit := repofile.Parse(string(files[path]))
		if unit == nil {
			continue
		}
		if err := repofile.Ingest(tree, unit, path); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

func (r *DoltRepository) ApplyKnowledgeCommit(cs knowledge.ChangeSet) (kernel.CommitID, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.archivedLocked() {
		return "", kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", r.repositoryID)
	}
	if err := knowledge.ValidateProvenance(cs.Provenance); err != nil {
		return "", err
	}
	if cs.TargetRepository != r.repositoryID {
		return "", kernel.Fail(kernel.ErrTargetRepositoryDenied, "target %s does not match %s", cs.TargetRepository, r.repositoryID)
	}
	if cs.BaseCommit != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "baseCommit must equal expectedTargetCommit")
	}
	branch, ok := doltBranch(cs.TargetRef)
	if !ok {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported Dolt ref %s", cs.TargetRef)
	}
	current, err := r.queryHash(branch)
	if err != nil || current != cs.ExpectedTargetCommit {
		return "", kernel.Fail(kernel.ErrNonFastForward, "ref %s moved: expected commit %s, actual %s", cs.TargetRef, cs.ExpectedTargetCommit, current)
	}
	tree, err := r.scanAtLocked(cs.BaseCommit)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, op := range cs.Operations {
		if err := repofile.Apply(tree, op, cs.Provenance, toWrite, toDelete); err != nil {
			return "", err
		}
	}
	changes := make([]snapshot.TreeChange, 0, len(toWrite)+len(toDelete))
	for path := range toDelete {
		if _, replaced := toWrite[path]; !replaced {
			changes = append(changes, snapshot.TreeChange{Path: path, Remove: true})
		}
	}
	for path, content := range toWrite {
		changes = append(changes, snapshot.TreeChange{Path: path, Content: []byte(content)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return r.applyTreeLocked(snapshot.TreeChangeSet{
		TargetRepository: r.repositoryID, TargetRef: cs.TargetRef,
		BaseCommit: cs.BaseCommit, ExpectedTargetCommit: cs.ExpectedTargetCommit,
		Changes: changes, Message: cs.Message, Author: cs.Author,
		RequestID: cs.RequestID, RuleID: cs.RuleID,
	})
}
