package writer

import (
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// commitReadBasis is an internal planning view, never the write authority.
// The fixed empty tree is independently established by Writer, not asserted
// by a ChangeSet or a client. Native object authorities keep their own reads.
func commitReadBasis(target snapshot.Store, at kernel.CommitID) (snapshot.Store, error) {
	if at == "" {
		return target, nil
	}
	if _, native := target.(objectmodel.NativeRepository); native {
		return target, nil
	}
	tree, ok := snapshot.TreeStoreOf(target)
	if !ok {
		return target, nil
	}
	empty, known, err := snapshot.ProbeEmptyTree(target, at)
	if err != nil {
		return nil, err
	}
	if !known || !empty {
		return target, nil
	}
	return &emptyTreeBasis{Store: target, TreeStore: tree, at: at}, nil
}

type emptyTreeBasis struct {
	snapshot.Store
	snapshot.TreeStore
	at kernel.CommitID
}

func (s *emptyTreeBasis) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if at == s.at {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "file %s does not exist at empty commit %s", path, at)
	}
	return s.TreeStore.ReadFile(path, at)
}

func (s *emptyTreeBasis) ListFiles(at kernel.CommitID) ([]string, error) {
	if at == s.at {
		return nil, nil
	}
	return s.TreeStore.ListFiles(at)
}
