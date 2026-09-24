package home

import (
	"kc/kernel"
	"kc/snapshot"
)

func requireTreeCapability(source snapshot.Store) (snapshot.TreeStore, error) {
	tree, ok := snapshot.TreeStoreOf(source)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"authority %s does not provide immutable tree access", source.ID())
	}
	return tree, nil
}

func requireDirectoryCapability(source snapshot.Store) (snapshot.DirectoryReader, error) {
	directory, ok := snapshot.DirectoryReaderOf(source)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"authority %s does not provide bounded directory access", source.ID())
	}
	return directory, nil
}

func requireHistoryCapability(source snapshot.Store) (snapshot.HistoryStore, error) {
	history, ok := snapshot.HistoryStoreOf(source)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"authority %s does not provide commit history", source.ID())
	}
	return history, nil
}

func requireChangeCapability(source snapshot.Store) (snapshot.ChangeStore, error) {
	changes, ok := snapshot.ChangeStoreOf(source)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"authority %s does not provide changed-path access", source.ID())
	}
	return changes, nil
}

func requireBulkCapability(source snapshot.Store) (snapshot.BulkTreeIngester, error) {
	bulk, ok := snapshot.BulkTreeIngesterOf(source)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"authority %s does not provide bulk ingest", source.ID())
	}
	return bulk, nil
}
