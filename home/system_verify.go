package home

import (
	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func verifySystemAuthority(source snapshot.Store) error {
	repo, err := reader.NewReader(nil).Wrap(source, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return err
	}
	head, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		return err
	}
	return verifySystemPublication(repo, source, head)
}

func verifySystemPublication(repo knowledge.ReadStore, source snapshot.Store, commit kernel.CommitID) error {
	operations := knowledge.SystemPublicationOperations()
	for _, expected := range operations {
		current, err := repo.ReadAddress(expected.Address, commit)
		if err != nil {
			return err
		}
		resolved, err := repo.ResolveAddress(expected.Address, commit)
		if err != nil {
			return err
		}
		actual := knowledge.Operation{Op: knowledge.OpPut, Address: current.Address, SchemaRef: resolved.SchemaRef, PathHint: resolved.PathHint, Value: current.Value}
		if kernel.CanonicalDigest(actual) != kernel.CanonicalDigest(expected) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "System publication differs from the binary publication at %s", knowledge.AddressKey(expected.Address))
		}
	}
	tree, ok := snapshot.TreeReaderOf(source)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "System publication requires literal file reads")
	}
	return repofile.AssertPublicationFiles(tree, commit, operations)
}
