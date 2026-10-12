package home

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// initializeSystemAuthority publishes only during an explicit deployment
// operation. An initialized deployment must restore its existing publication.
func initializeSystemAuthority(store snapshot.Registry, c DeploymentConfig, initialized bool) error {
	binding, err := c.Binding(knowledge.SystemRepositoryID)
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
	} else if _, err := writer.PublishSystem(source); err != nil {
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
	return &systemAuthority{source, tree, directory, history, changes}, nil
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
