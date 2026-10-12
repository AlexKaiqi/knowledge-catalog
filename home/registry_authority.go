package home

import (
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func closeRegistryStore(store snapshot.Store) {
	if store == nil {
		return
	}
	if closer, ok := store.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func openRegistry(binding RegistryBinding) (dataset.Registry, error) {
	store, err := openRegistryAuthority(binding)
	if err != nil {
		return nil, err
	}
	registry, err := dataset.OpenSnapshotRegistry(store, binding.Ref)
	if err != nil {
		closeRegistryStore(store)
		return nil, err
	}
	return registry, nil
}

func createRegistry(binding RegistryBinding) (dataset.Registry, error) {
	store, err := openRegistryAuthority(binding)
	if err != nil {
		return nil, err
	}
	registry, err := dataset.CreateSnapshotRegistry(store, binding.Ref)
	if err != nil {
		closeRegistryStore(store)
		return nil, err
	}
	return registry, nil
}

// PrepareRegistryAuthority verifies the declared registry Snapshot location.
// Explicit deployment init creates the registry ref and its membership.
func PrepareRegistryAuthority(binding RegistryBinding) error {
	store, err := openRegistryAuthority(binding)
	if err != nil {
		return err
	}
	closeRegistryStore(store)
	return nil
}
