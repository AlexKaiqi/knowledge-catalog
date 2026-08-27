package index

import "kc/kernel"

// engineKey identifies either a live engine (empty commit) or a frozen pin engine.
type engineKey struct {
	repo   kernel.RepositoryID
	commit kernel.CommitID
	lane   string
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
	return idx.engineLaneAt(id, commit, "")
}

func (idx *Index) engineLaneAt(id kernel.RepositoryID, commit kernel.CommitID, lane string) (Engine, error) {
	key := engineKey{repo: id, commit: commit, lane: lane}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if eng, ok := idx.engs[key]; ok {
		return eng, nil
	}
	openID := pinOpenID(id, commit)
	if lane != "" {
		openID = kernel.RepositoryID(string(openID) + "#" + lane)
	}
	eng, err := idx.open(idx.dir, openID)
	if err != nil {
		return nil, err
	}
	idx.engs[key] = eng
	return eng, nil
}

func (idx *Index) stateEngineAt(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	return idx.engineLaneAt(id, commit, "state")
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
