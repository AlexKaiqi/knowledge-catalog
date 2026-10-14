package home

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/systemrepo"
)

// initializeSystemAuthority publishes only during an explicit deployment
// operation. An initialized deployment must restore its existing publication.
func initializeSystemAuthority(store snapshot.Registry, c DeploymentConfig, initialized bool) error {
	binding, err := c.Binding(objectmodel.SystemRepositoryID)
	if err != nil {
		return err
	}
	var source snapshot.Store
	if initialized {
		source, err = openExistingAuthority(binding.homeRepo())
	} else {
		source, err = openAuthority(binding.Dir, binding.homeRepo())
	}
	if err != nil {
		return err
	}
	owned := false
	defer func() {
		if !owned {
			closeManagedSource(source)
		}
	}()
	if initialized {
		if err := verifySystemAuthority(source); err != nil {
			return err
		}
	} else if _, err := systemrepo.PublishSystem(source); err != nil {
		return err
	}
	// Registry registration only needs the identity and existing version.
	if err := store.Add(source); err != nil {
		return err
	}
	owned = true
	return nil
}

// systemAuthority retains actual Snapshot reads while excluding raw mutation
// capabilities. All supported system drivers provide these read ports.
type systemAuthority struct {
	snapshot.Store
	snapshot.TreeReader
	snapshot.DirectoryReader
	snapshot.HistoryStore
	snapshot.ChangeStore
}

type nativeRepositoryMetadataReader interface {
	ReadRepositoryMetadata() (map[string]string, error)
}

// Metadata remains optional and read-only on the runtime System authority.
// Embedding just the read port does not expose the source's native setter.
type systemMetadataAuthority struct {
	*systemAuthority
	nativeRepositoryMetadataReader
}

func readOnlySystemAuthority(source snapshot.Store) (snapshot.Store, error) {
	tree, ok := snapshot.TreeReaderOf(source)
	if !ok {
		closeManagedSource(source)
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "System Repository requires literal file reads")
	}
	directory, err := requireDirectoryCapability(source)
	if err != nil {
		closeManagedSource(source)
		return nil, err
	}
	history, err := requireHistoryCapability(source)
	if err != nil {
		closeManagedSource(source)
		return nil, err
	}
	changes, err := requireChangeCapability(source)
	if err != nil {
		closeManagedSource(source)
		return nil, err
	}
	authority := &systemAuthority{source, tree, directory, history, changes}
	if metadata, ok := source.(nativeRepositoryMetadataReader); ok {
		return &systemMetadataAuthority{authority, metadata}, nil
	}
	return authority, nil
}

func (*systemAuthority) CreateRef(string, kernel.CommitID) error {
	return kernel.Fail(kernel.ErrForbidden, "System Repository is read-only at runtime")
}

func (*systemAuthority) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrForbidden, "System Repository is read-only at runtime")
}

func (*systemAuthority) Archive() error {
	return kernel.Fail(kernel.ErrForbidden, "System Repository is read-only at runtime")
}

func (s *systemAuthority) Origin() string {
	if source, ok := s.Store.(snapshot.StoreOrigin); ok {
		return source.Origin()
	}
	return ""
}

func (s *systemAuthority) Close() error {
	if source, ok := s.Store.(interface{ Close() error }); ok {
		return source.Close()
	}
	return nil
}
