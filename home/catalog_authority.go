package home

import (
	"kc/catalog"
	"kc/snapshot"
)

func closeCatalogStore(store snapshot.Store) {
	if store == nil {
		return
	}
	if closer, ok := store.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func openCatalogRegistry(binding CatalogBinding) (*catalog.Registry, error) {
	store, err := openCatalogAuthority(binding, false)
	if err != nil {
		return nil, err
	}
	registry, err := catalog.OpenSnapshotRegistry(store, binding.ID, binding.Ref)
	if err != nil {
		closeCatalogStore(store)
		return nil, err
	}
	return registry, nil
}

func createCatalogRegistry(binding CatalogBinding) (*catalog.Registry, error) {
	store, err := openCatalogAuthority(binding, true)
	if err != nil {
		return nil, err
	}
	registry, err := catalog.CreateSnapshotRegistry(store, binding.ID, binding.Ref)
	if err != nil {
		closeCatalogStore(store)
		return nil, err
	}
	return registry, nil
}

// PrepareCatalogAuthority provisions an empty Snapshot container for a Catalog.
// It does not write Catalog membership; deployment init creates the registry ref.
func PrepareCatalogAuthority(binding CatalogBinding) error {
	store, err := openCatalogAuthority(binding, true)
	if err != nil {
		return err
	}
	closeCatalogStore(store)
	return nil
}
