package writer

import (
	"encoding/json"
	"sort"
	"strings"

	"kc/kernel"
	repofile "kc/object/format"
	"kc/snapshot"
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
		"operation": "rebuild-object-locators", "repository": repository,
		"targetRef": targetRef, "expected": expected,
	}))
	entry, replayed, err := w.commands.Execute(commandID, digest, snapshot.CommandRequest{
		Kind: "OBJECT_LOCATOR_REBUILD", RepositoryID: string(repository),
		TargetRef: targetRef, BaseCommit: string(expected), ExpectedTargetCommit: string(expected),
	}, func() (_ any, err error) {
		writeStarted := false
		defer func() {
			if err != nil && !writeStarted {
				err = snapshot.WriteNotApplied(err)
			}
		}()
		head, err := target.Head(targetRef)
		if err != nil {
			return nil, err
		}
		if head != expected {
			return nil, kernel.Fail(kernel.ErrNonFastForward,
				"locator rebuild expected %s, current %s", expected, head)
		}
		index, err := repofile.ReadTree(tree, expected)
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
				strings.HasPrefix(name, repofile.LocatorSchemaReferrersDirectory+"/") ||
				name == repofile.LocatorManifestPath || name == repofile.LocatorCompletePath ||
				name == repofile.LocatorSchemaIndexPath || name == repofile.LocatorBindingIndexPath ||
				name == repofile.LocatorReferrersCompletePath {
				changes = append(changes, snapshot.TreeChange{Path: name, Remove: true})
			}
		}
		for objectID, units := range index.ByObject {
			raw, err := encodeUnitLocator(objectID, units)
			if err != nil {
				return nil, err
			}
			changes = append(changes, snapshot.TreeChange{
				Path: repofile.ObjectLocatorPath(objectID), Content: raw,
			})
		}
		manifest := repofile.BuildLocatorManifest(index)
		schemas, err := repofile.EncodeSchemaIndex(manifest.Schemas)
		if err != nil {
			return nil, err
		}
		bindings, err := repofile.EncodeBindingIndex(manifest.Bindings)
		if err != nil {
			return nil, err
		}
		changes = append(changes,
			snapshot.TreeChange{Path: repofile.LocatorSchemaIndexPath, Content: schemas},
			snapshot.TreeChange{Path: repofile.LocatorBindingIndexPath, Content: bindings})
		for schema, addresses := range manifest.Referrers {
			encoded, err := repofile.EncodeSchemaReferrers(schema, addresses)
			if err != nil {
				return nil, err
			}
			changes = append(changes, snapshot.TreeChange{Path: repofile.SchemaReferrersPath(schema), Content: encoded})
		}
		changes = append(changes, snapshot.TreeChange{
			Path: repofile.LocatorReferrersCompletePath, Content: []byte(repofile.LocatorReferrersCompleteBody),
		})
		changes = append(changes, snapshot.TreeChange{
			Path: repofile.LocatorCompletePath, Content: []byte(repofile.LocatorCompleteBody),
		})
		changes = collapseRebuildChanges(changes)
		writeStarted = true
		next, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
			TargetRepository: repository, TargetRef: targetRef,
			BaseCommit: expected, ExpectedTargetCommit: expected,
			Changes: changes, Message: "rebuild bounded object locators",
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

// collapseRebuildChanges keeps one change per path: a rewritten locator file
// replaces its removal, so the rebuilt layout never depends on change order.
func collapseRebuildChanges(changes []snapshot.TreeChange) []snapshot.TreeChange {
	byPath := make(map[string]snapshot.TreeChange, len(changes))
	for _, change := range changes {
		if previous, seen := byPath[change.Path]; seen && !previous.Remove && change.Remove {
			continue
		}
		byPath[change.Path] = change
	}
	out := make([]snapshot.TreeChange, 0, len(byPath))
	for _, change := range byPath {
		out = append(out, change)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
