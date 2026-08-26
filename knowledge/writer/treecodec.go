package writer

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// applyKnowledgeCommit is the sole ② → ⓪ codec. Snapshot adapters receive
// only literal path changes and never see Address, Aspect, or PUT/REMOVE.
func applyKnowledgeCommit(target snapshot.Store, cs knowledge.ChangeSet) (kernel.CommitID, error) {
	tree, ok := snapshot.TreeStoreOf(target)
	if !ok {
		return "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s is mounted as a plain snapshot and cannot accept knowledge changes without tree access", target.ID())
	}

	idx, err := readKnowledgeTree(tree, cs.BaseCommit)
	if err != nil {
		return "", err
	}
	toWrite := map[string]string{}
	toDelete := map[string]struct{}{}
	for _, op := range cs.Operations {
		if err := repofile.Apply(idx, op, cs.Provenance, toWrite, toDelete); err != nil {
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

	return tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository:     cs.TargetRepository,
		TargetRef:            cs.TargetRef,
		BaseCommit:           cs.BaseCommit,
		ExpectedTargetCommit: cs.ExpectedTargetCommit,
		Changes:              changes,
		Message:              cs.Message,
		Author:               cs.Author,
		RequestID:            cs.RequestID,
		RuleID:               cs.RuleID,
	})
}

func readKnowledgeTree(tree snapshot.TreeStore, commit kernel.CommitID) (*repofile.Tree, error) {
	paths, err := tree.ListFiles(commit)
	if err != nil {
		return nil, err
	}
	idx := repofile.NewTree()
	for _, path := range paths {
		if !repofile.KnowledgePath(path) {
			continue
		}
		content, err := tree.ReadFile(path, commit)
		if err != nil {
			return nil, err
		}
		unit := repofile.Parse(string(content))
		if unit == nil {
			continue
		}
		if err := repofile.Ingest(idx, unit, path); err != nil {
			return nil, err
		}
	}
	return idx, nil
}
