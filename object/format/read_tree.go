package format

import (
	"kc/kernel"
	"kc/snapshot"
)

func ReadTree(tree snapshot.TreeReader, commit kernel.CommitID) (*Tree, error) {
	paths, err := tree.ListFiles(commit)
	if err != nil {
		return nil, err
	}
	idx := NewTree()
	for _, path := range paths {
		if !KnowledgePath(path) {
			continue
		}
		content, err := tree.ReadFile(path, commit)
		if err != nil {
			return nil, err
		}
		unit := Parse(string(content))
		if unit == nil {
			continue
		}
		if err := Ingest(idx, unit, path); err != nil {
			return nil, err
		}
	}
	return idx, nil
}
