package writer

import (
	"encoding/json"
	"sort"
	"strings"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/commandlog"
)

type LocatorRebuildReceipt struct {
	CommandID   string              `json:"commandId"`
	Replayed    bool                `json:"replayed"`
	Repository  kernel.RepositoryID `json:"repository"`
	OldCommit   kernel.CommitID     `json:"oldCommit"`
	NewCommit   kernel.CommitID     `json:"newCommit"`
	ObjectCount int                 `json:"objectCount"`
}

// RebuildTreeLocators is an explicit maintenance scan for a legacy or damaged
// file-backed authority. Consumer reads and ordinary writes never invoke it.
func (w *Writer) RebuildTreeLocators(commandID string, repository kernel.RepositoryID, targetRef string, expected kernel.CommitID) (LocatorRebuildReceipt, error) {
	if strings.TrimSpace(commandID) == "" || repository == "" || expected == "" {
		return LocatorRebuildReceipt{}, kernel.Fail(kernel.ErrUsageInvalid,
			"locator rebuild requires command, repository, and expected commit")
	}
	targetRef = snapshot.RefOrDefault(targetRef)
	target, err := w.store.Require(repository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return LocatorRebuildReceipt{}, err
	}
	tree, ok := snapshot.TreeStoreOf(target)
	if !ok {
		return LocatorRebuildReceipt{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s has no file locator maintenance capability", repository)
	}
	digest := string(kernel.CanonicalDigest(map[string]any{
		"operation": "rebuild-knowledge-locators", "repository": repository,
		"targetRef": targetRef, "expected": expected,
	}))
	entry, replayed, err := w.commands.Execute(commandID, digest, commandlog.Request{
		Kind: "KNOWLEDGE_LOCATOR_REBUILD", RepositoryID: string(repository),
		TargetRef: targetRef, BaseCommit: string(expected), ExpectedTargetCommit: string(expected),
	}, func() (any, error) {
		head, err := target.Head(targetRef)
		if err != nil {
			return nil, err
		}
		if head != expected {
			return nil, kernel.Fail(kernel.ErrNonFastForward,
				"locator rebuild expected %s, current %s", expected, head)
		}
		index, err := readKnowledgeTree(tree, expected)
		if err != nil {
			return nil, err
		}
		existing, err := tree.ListFiles(expected)
		if err != nil {
			return nil, err
		}
		changes := []snapshot.TreeChange{}
		for _, name := range existing {
			if strings.HasPrefix(name, repofile.LocatorObjectDirectory+"/") ||
				name == repofile.LocatorManifestPath || name == repofile.LocatorCompletePath ||
				name == repofile.LocatorSchemaIndexPath {
				changes = append(changes, snapshot.TreeChange{Path: name, Remove: true})
			}
		}
		for objectID, units := range index.ByObject {
			paths := make([]string, 0, len(units))
			for _, unit := range units {
				paths = append(paths, unit.Path)
			}
			raw, err := repofile.EncodeObjectLocator(objectID, paths)
			if err != nil {
				return nil, err
			}
			changes = append(changes, snapshot.TreeChange{
				Path: repofile.ObjectLocatorPath(objectID), Content: raw,
			})
		}
		schemas, err := repofile.EncodeSchemaIndex(repofile.BuildLocatorManifest(index).Schemas)
		if err != nil {
			return nil, err
		}
		changes = append(changes, snapshot.TreeChange{
			Path: repofile.LocatorSchemaIndexPath, Content: schemas,
		})
		changes = append(changes, snapshot.TreeChange{
			Path: repofile.LocatorCompletePath, Content: []byte(repofile.LocatorCompleteBody),
		})
		sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
		next, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
			TargetRepository: repository, TargetRef: targetRef,
			BaseCommit: expected, ExpectedTargetCommit: expected,
			Changes: changes, Message: "rebuild bounded knowledge locators",
			RequestID: commandID,
		})
		if err != nil {
			return nil, err
		}
		w.store.NotifyAdvanced(snapshot.Advanced{Store: target, From: expected, To: next})
		return LocatorRebuildReceipt{
			CommandID: commandID, Repository: repository,
			OldCommit: expected, NewCommit: next, ObjectCount: len(index.ByObject),
		}, nil
	})
	if err != nil {
		return LocatorRebuildReceipt{}, err
	}
	var receipt LocatorRebuildReceipt
	if err := json.Unmarshal(entry.Receipt, &receipt); err != nil {
		return LocatorRebuildReceipt{}, err
	}
	receipt.Replayed = replayed
	return receipt, nil
}
