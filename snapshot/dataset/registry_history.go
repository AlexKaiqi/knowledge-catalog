package dataset

import (
	"bytes"

	"kc/kernel"
	"kc/snapshot"
)

func (g *registry) snapshotHistory(limit int, path string) ([]Commit, error) {
	if g.head == "" {
		return []Commit{}, nil
	}
	history, ok := snapshot.HistoryStoreOf(g.authority)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "registry authority does not support commit history")
	}
	if limit <= 0 {
		limit = 20
	}
	readLimit := max(2, min(limit+1, 128))
	start := kernel.CommitID(g.head)
	out := []Commit{}
	for {
		ids, err := history.CommitHistory(start, readLimit)
		if err != nil {
			return nil, err
		}
		// A short provider page is not proof of exhaustion. Keep its last
		// commit as the next immutable anchor to obtain the missing parent.
		exhausted := len(ids) <= 1
		page, err := g.filterSnapshotHistory(ids, path, limit-len(out), exhausted)
		out = append(out, page...)
		if err != nil || len(out) == limit || exhausted {
			return out, err
		}
		start = ids[len(ids)-1]
	}
}

func (g *registry) filterSnapshotHistory(ids []kernel.CommitID, path string, limit int, exhausted bool) ([]Commit, error) {
	out := []Commit{}
	for i, id := range ids {
		if i == len(ids)-1 && !exhausted {
			break
		}
		// A provisioned authority may precede its first Service commit.
		// Its empty root is not an accepted registry history entry.
		metadata, err := g.registryFileAt(RegistryFile(), id)
		if err != nil {
			return nil, err
		}
		if len(metadata) == 0 {
			metadata, err = g.registryFileAt("catalog.yaml", id)
			if err != nil {
				return nil, err
			}
			if len(metadata) == 0 {
				continue
			}
		}
		if path != "" {
			var parent kernel.CommitID
			if i+1 < len(ids) {
				parent = ids[i+1]
			} else if !exhausted {
				break
			}
			changed, err := g.snapshotPathChanged(path, parent, id)
			if err != nil {
				return nil, err
			}
			if !changed {
				continue
			}
		}
		entry, err := g.snapshotCommit(id)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (g *registry) snapshotPathChanged(path string, parent, commit kernel.CommitID) (bool, error) {
	if changes, ok := snapshot.ChangeStoreOf(g.authority); ok && parent != "" {
		paths, err := changes.ChangedPaths(parent, commit)
		if err != nil {
			return false, err
		}
		for _, changed := range paths {
			if changed == path {
				return true, nil
			}
		}
		return false, nil
	}
	current, err := g.registryFileAt(path, commit)
	if err != nil {
		return false, err
	}
	older, err := g.registryFileAt(path, parent)
	return !bytes.Equal(current, older), err
}

func (g *registry) registryFileAt(path string, commit kernel.CommitID) ([]byte, error) {
	if commit == "" {
		return nil, nil
	}
	body, err := g.tree.ReadFile(path, commit)
	if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
		return nil, nil
	}
	return body, err
}

func (g *registry) snapshotCommit(commit kernel.CommitID) (Commit, error) {
	entry := Commit{Commit: string(commit)}
	if reader, ok := g.authority.(snapshot.CommitMetadataReader); ok {
		meta, err := reader.ReadCommitMetadata(commit)
		if err != nil {
			return Commit{}, err
		}
		entry.Author, entry.Message = meta.Author, meta.Message
		entry.RequestID, entry.RuleID = meta.RequestID, meta.RuleID
	}
	return entry, nil
}
