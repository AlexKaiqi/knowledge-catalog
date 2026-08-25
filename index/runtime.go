package index

import "kc/kernel"

// engineKey identifies either a live engine (empty commit) or a frozen pin engine.
type engineKey struct {
	repo   kernel.RepositoryID
	commit kernel.CommitID
}

func (idx *Index) engine(id kernel.RepositoryID) (Engine, error) {
	return idx.engineAt(id, "")
}

func pinOpenID(id kernel.RepositoryID, commit kernel.CommitID) kernel.RepositoryID {
	if commit == "" {
		return id
	}
	return kernel.RepositoryID(string(id) + "@" + string(commit))
}

func (idx *Index) engineAt(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	key := engineKey{repo: id, commit: commit}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if eng, ok := idx.engs[key]; ok {
		return eng, nil
	}
	eng, err := idx.open(idx.dir, pinOpenID(id, commit))
	if err != nil {
		return nil, err
	}
	idx.engs[key] = eng
	return eng, nil
}

func (idx *Index) engineForCommit(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	live, err := idx.engine(id)
	if err != nil {
		return nil, err
	}
	meta, err := live.LoadMeta()
	if err != nil {
		return nil, err
	}
	if meta.Basis == commit {
		return live, nil
	}
	return idx.engineAt(id, commit)
}
