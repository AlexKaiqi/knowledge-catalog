package writer

import (
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

const systemPublishCommandPrefix = "kc.system.publish:"

// PublishResult is one attempt to place the built-in System Schema onto a
// Snapshot authority identified as kr://kc/system.
type PublishResult struct {
	Commit kernel.CommitID
	Seeded bool
}

type systemPublication byte

const (
	systemPublicationEmpty systemPublication = iota
	systemPublicationMatch
)

// PublishSystem writes the binary System Schema objects onto an empty
// Snapshot authority whose ID is kr://kc/system. Occupied authorities are
// verified against the built-in trust root and never overwritten.
func PublishSystem(store snapshot.Store) (PublishResult, error) {
	if store == nil || store.ID() != knowledge.SystemRepositoryID {
		return PublishResult{}, kernel.Fail(kernel.ErrUsageInvalid,
			"system publish requires repository %s", knowledge.SystemRepositoryID)
	}
	operations := knowledge.SystemSchemaOperations()
	head, err := store.Head(snapshot.DefaultRef)
	if err != nil {
		return PublishResult{}, err
	}
	state, err := inspectSystemPublication(store, head, operations)
	if err != nil {
		return PublishResult{}, err
	}
	if state == systemPublicationMatch {
		return PublishResult{Commit: head, Seeded: false}, nil
	}

	registry := snapshot.NewRegistry()
	if err := registry.Add(store); err != nil {
		return PublishResult{}, err
	}
	w, err := NewWriter(registry, nil)
	if err != nil {
		return PublishResult{}, err
	}
	w.SetStamp("kc-system", "", "")
	receipt, err := w.Commit(systemPublishCommandPrefix+string(kernel.CanonicalDigest(operations)), knowledge.CommitChangeSet{
		TargetRepository:     knowledge.SystemRepositoryID,
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Operations:           operations,
		Message:              "Publish built-in System Schema",
	})
	if err != nil {
		return PublishResult{}, err
	}
	return PublishResult{Commit: receipt.Result.CommitID, Seeded: true}, nil
}

func inspectSystemPublication(store snapshot.Store, head kernel.CommitID, operations []knowledge.Operation) (systemPublication, error) {
	if native, ok := store.(knowledge.NativeRepository); ok {
		return inspectNativeSystemPublication(native, head, operations)
	}
	tree, ok := snapshot.TreeStoreOf(store)
	if !ok {
		return 0, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s cannot accept System Schema publication", store.ID())
	}
	index, err := readKnowledgeTree(tree, head)
	if err != nil {
		return 0, err
	}
	return inspectSystemValues(head, operations, func(objectID knowledge.ObjectID) (any, error) {
		units := index.ObjectUnits(objectID)
		if len(units) == 0 {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
				"system object %s is missing at commit %s", objectID, head)
		}
		return repofile.Assemble(units)
	})
}

func inspectNativeSystemPublication(native knowledge.NativeRepository, head kernel.CommitID, operations []knowledge.Operation) (systemPublication, error) {
	return inspectSystemValues(head, operations, func(objectID knowledge.ObjectID) (any, error) {
		current, err := native.Read(objectID, head)
		if err != nil {
			return nil, err
		}
		return current.Value, nil
	})
}

func inspectSystemValues(head kernel.CommitID, operations []knowledge.Operation, value func(knowledge.ObjectID) (any, error)) (systemPublication, error) {
	missing := 0
	for _, operation := range operations {
		current, err := value(operation.Address.ObjectID)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				missing++
				continue
			}
			return 0, err
		}
		got, want := kernel.CanonicalDigest(current), kernel.CanonicalDigest(operation.Value)
		if got != want {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed,
				"system schema %s digest %s does not match built-in trust root %s",
				operation.Address.ObjectID, got, want)
		}
	}
	switch missing {
	case 0:
		return systemPublicationMatch, nil
	case len(operations):
		return systemPublicationEmpty, nil
	default:
		return 0, kernel.Fail(kernel.ErrPreconditionFailed,
			"System Repository is partially published at commit %s and does not match the built-in trust root", head)
	}
}
