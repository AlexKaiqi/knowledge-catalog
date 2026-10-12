package writer

import (
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

const systemPublishCommandPrefix = "kc.system.publish:"

// PublishResult is one attempt to place the system publication source onto a
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

// PublishSystem writes the complete binary system publication onto an empty
// Snapshot authority whose ID is kr://kc/system. Occupied authorities are
// verified against the built-in trust root and never overwritten.
func PublishSystem(store snapshot.Store) (PublishResult, error) {
	if store == nil || store.ID() != knowledge.SystemRepositoryID {
		return PublishResult{}, kernel.Fail(kernel.ErrUsageInvalid,
			"system publish requires repository %s", knowledge.SystemRepositoryID)
	}
	operations := knowledge.SystemPublicationOperations()
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
	w.systemPublish = true
	receipt, err := w.Commit(systemPublishCommandPrefix+string(kernel.CanonicalDigest(operations)), knowledge.CommitChangeSet{
		TargetRepository:     knowledge.SystemRepositoryID,
		TargetRef:            snapshot.DefaultRef,
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Operations:           operations,
		Message:              "Publish system knowledge",
	})
	if err != nil {
		return PublishResult{}, err
	}
	return PublishResult{Commit: receipt.Result.CommitID, Seeded: true}, nil
}

func inspectSystemPublication(store snapshot.Store, head kernel.CommitID, operations []knowledge.Operation) (systemPublication, error) {
	state, err := inspectSystemUnits(store, head, operations)
	if err != nil {
		return 0, err
	}
	tree, ok := snapshot.TreeReaderOf(store)
	if !ok {
		return 0, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "System publication requires literal file reads")
	}
	expected := operations
	if state == systemPublicationEmpty {
		expected = nil
	}
	if err := repofile.AssertPublicationFiles(tree, head, expected); err != nil {
		return 0, err
	}
	return state, nil
}

func inspectSystemUnits(store snapshot.Store, head kernel.CommitID, operations []knowledge.Operation) (systemPublication, error) {
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
	return inspectSystemValues(head, operations, func(address knowledge.Address) (any, error) {
		for _, unit := range index.ObjectUnits(address.ObjectID) {
			if knowledge.AddressKey(unit.Address) == knowledge.AddressKey(address) {
				return knowledge.Operation{Op: knowledge.OpPut, Address: unit.Address, PathHint: unit.PathHint, SchemaRef: unit.SchemaRef, Value: unit.Value}, nil
			}
		}
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
			"system address %s is missing at commit %s", knowledge.AddressKey(address), head)
	})
}

func inspectNativeSystemPublication(native knowledge.NativeRepository, head kernel.CommitID, operations []knowledge.Operation) (systemPublication, error) {
	return inspectSystemValues(head, operations, func(address knowledge.Address) (any, error) {
		current, err := native.ReadAddress(address, head)
		if err != nil {
			return nil, err
		}
		resolved, err := native.ResolveAddress(address, head)
		if err != nil {
			return nil, err
		}
		return knowledge.Operation{Op: knowledge.OpPut, Address: current.Address, PathHint: resolved.PathHint, SchemaRef: resolved.SchemaRef, Value: current.Value}, nil
	})
}

func inspectSystemValues(head kernel.CommitID, operations []knowledge.Operation, value func(knowledge.Address) (any, error)) (systemPublication, error) {
	missing := 0
	for _, operation := range operations {
		current, err := value(operation.Address)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				missing++
				continue
			}
			return 0, err
		}
		got, want := kernel.CanonicalDigest(current), kernel.CanonicalDigest(operation)
		if got != want {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed,
				"system address %s digest %s does not match the binary publication %s",
				knowledge.AddressKey(operation.Address), got, want)
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
